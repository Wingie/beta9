package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// ============================================================================
// Inference Router - Routes inference requests to nodes
// ============================================================================

// ChatRequest is the incoming chat request format (OpenAI-compatible)
type ChatRequest struct {
	Model       string         `json:"model"`
	Messages    []ChatMessage  `json:"messages"`
	Temperature float64        `json:"temperature,omitempty"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
	Stream      bool           `json:"stream,omitempty"`
	PreferGPU   string         `json:"prefer_gpu,omitempty"` // "MPS", "CUDA", etc.
}

// ChatMessage represents a chat message
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse is the response format
type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Node string `json:"node,omitempty"` // Which node handled the request
}

// EmbedRequest is the incoming embed request
type EmbedRequest struct {
	Model     string   `json:"model"`
	Input     []string `json:"input"`
	PreferGPU string   `json:"prefer_gpu,omitempty"`
}

// EmbedResponse is the embedding response
type EmbedResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	Node string `json:"node,omitempty"`
}

// RouterError represents an error during routing
type RouterError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *RouterError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// InferenceRouter routes inference requests to appropriate nodes
type InferenceRouter struct {
	registry *ModelRegistry
	client   *http.Client
}

// NewInferenceRouter creates a new InferenceRouter
func NewInferenceRouter(registry *ModelRegistry) *InferenceRouter {
	return &InferenceRouter{
		registry: registry,
		client: &http.Client{
			Timeout: 5 * time.Minute, // Long timeout for inference
		},
	}
}

// Route finds the best node for a model request
func (r *InferenceRouter) Route(model string, preferGPU string) (*NodeInferenceInfo, error) {
	node := r.registry.FindNodeForModel(model, preferGPU)
	if node == nil {
		return nil, &RouterError{
			Code:    "NO_NODE_AVAILABLE",
			Message: fmt.Sprintf("no healthy node available for model %s", model),
		}
	}
	return node, nil
}

// Chat routes a chat request to the appropriate node
func (r *InferenceRouter) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// Find node for the model
	node, err := r.Route(req.Model, req.PreferGPU)
	if err != nil {
		return nil, err
	}

	log.Info().
		Str("model", req.Model).
		Str("node", node.NodeID).
		Str("tailscale_ip", node.TailscaleIP).
		Int("port", node.Port).
		Msg("Routing chat request to node")

	// Forward to node's Ollama instance
	ollamaReq := map[string]interface{}{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   false, // TODO: Add streaming support
	}
	if req.Temperature > 0 {
		ollamaReq["options"] = map[string]interface{}{
			"temperature": req.Temperature,
		}
	}
	if req.MaxTokens > 0 {
		if opts, ok := ollamaReq["options"].(map[string]interface{}); ok {
			opts["num_predict"] = req.MaxTokens
		} else {
			ollamaReq["options"] = map[string]interface{}{
				"num_predict": req.MaxTokens,
			}
		}
	}

	body, _ := json.Marshal(ollamaReq)
	url := fmt.Sprintf("http://%s:%d/api/chat", node.TailscaleIP, node.Port)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(httpReq)
	if err != nil {
		// Mark node as potentially unhealthy
		log.Error().Err(err).Str("node", node.NodeID).Msg("Node request failed")
		return nil, fmt.Errorf("node request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &RouterError{
			Code:    "NODE_ERROR",
			Message: fmt.Sprintf("node returned %d: %s", resp.StatusCode, string(respBody)),
		}
	}

	// Parse Ollama response
	var ollamaResp struct {
		Model   string `json:"model"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		Done               bool  `json:"done"`
		TotalDuration      int64 `json:"total_duration"`
		PromptEvalCount    int   `json:"prompt_eval_count"`
		EvalCount          int   `json:"eval_count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Update request count
	r.registry.IncrementRequestCount(node.NodeID, req.Model)

	// Convert to OpenAI-compatible format
	return &ChatResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   ollamaResp.Model,
		Choices: []struct {
			Index   int `json:"index"`
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		}{
			{
				Index: 0,
				Message: struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{
					Role:    ollamaResp.Message.Role,
					Content: ollamaResp.Message.Content,
				},
				FinishReason: "stop",
			},
		},
		Usage: struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		}{
			PromptTokens:     ollamaResp.PromptEvalCount,
			CompletionTokens: ollamaResp.EvalCount,
			TotalTokens:      ollamaResp.PromptEvalCount + ollamaResp.EvalCount,
		},
		Node: node.NodeID,
	}, nil
}

// Embed routes an embedding request to the appropriate node
func (r *InferenceRouter) Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
	// Find node for the model
	node, err := r.Route(req.Model, req.PreferGPU)
	if err != nil {
		return nil, err
	}

	log.Info().
		Str("model", req.Model).
		Str("node", node.NodeID).
		Int("inputs", len(req.Input)).
		Msg("Routing embed request to node")

	// Ollama expects single string for embed
	// We'll batch process if needed
	var allEmbeddings []struct {
		Object    string    `json:"object"`
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	}

	totalTokens := 0

	for i, input := range req.Input {
		ollamaReq := map[string]interface{}{
			"model":  req.Model,
			"prompt": input,
		}

		body, _ := json.Marshal(ollamaReq)
		url := fmt.Sprintf("http://%s:%d/api/embeddings", node.TailscaleIP, node.Port)

		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := r.client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("node request failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, &RouterError{
				Code:    "NODE_ERROR",
				Message: fmt.Sprintf("node returned %d: %s", resp.StatusCode, string(respBody)),
			}
		}

		var ollamaResp struct {
			Embedding []float64 `json:"embedding"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		resp.Body.Close()

		allEmbeddings = append(allEmbeddings, struct {
			Object    string    `json:"object"`
			Embedding []float64 `json:"embedding"`
			Index     int       `json:"index"`
		}{
			Object:    "embedding",
			Embedding: ollamaResp.Embedding,
			Index:     i,
		})

		// Rough token estimate
		totalTokens += len(input) / 4
	}

	// Update request count
	r.registry.IncrementRequestCount(node.NodeID, req.Model)

	return &EmbedResponse{
		Object: "list",
		Data:   allEmbeddings,
		Model:  req.Model,
		Usage: struct {
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		}{
			PromptTokens: totalTokens,
			TotalTokens:  totalTokens,
		},
		Node: node.NodeID,
	}, nil
}

// ListModels returns all available models across the cluster
func (r *InferenceRouter) ListModels() map[string][]ModelLocation {
	modelNodes := r.registry.ListModels()

	result := make(map[string][]ModelLocation)
	for model, nodes := range modelNodes {
		for _, node := range nodes {
			if modelInfo, exists := node.Models[model]; exists {
				result[model] = append(result[model], ModelLocation{
					NodeID:      node.NodeID,
					TailscaleIP: node.TailscaleIP,
					Port:        node.Port,
					GPUType:     node.GPUType,
					LoadState:   modelInfo.LoadState,
					LastUsed:    modelInfo.LastUsed,
				})
			}
		}
	}
	return result
}

// ModelLocation describes where a model is available
type ModelLocation struct {
	NodeID      string    `json:"node_id"`
	TailscaleIP string    `json:"tailscale_ip"`
	Port        int       `json:"port"`
	GPUType     string    `json:"gpu_type"`
	LoadState   LoadState `json:"load_state"`
	LastUsed    time.Time `json:"last_used"`
}

// Health returns router health status
func (r *InferenceRouter) Health() map[string]interface{} {
	stats := r.registry.Stats()
	stats["router"] = "healthy"
	return stats
}

// LoadModel requests a specific node to load a model
func (r *InferenceRouter) LoadModel(ctx context.Context, nodeID, model string) error {
	node := r.registry.GetNode(nodeID)
	if node == nil {
		return &RouterError{
			Code:    "NODE_NOT_FOUND",
			Message: fmt.Sprintf("node %s not found", nodeID),
		}
	}

	log.Info().
		Str("model", model).
		Str("node", nodeID).
		Msg("Loading model on node")

	// Send load request to node's control API
	ollamaReq := map[string]interface{}{
		"model":      model,
		"prompt":     "",
		"keep_alive": -1, // Keep loaded indefinitely
	}

	body, _ := json.Marshal(ollamaReq)
	url := fmt.Sprintf("http://%s:%d/api/generate", node.TailscaleIP, node.Port)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("node request failed: %w", err)
	}
	defer resp.Body.Close()

	// Drain response (streaming)
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		r.registry.MarkModelError(nodeID, model, fmt.Sprintf("load failed: %d", resp.StatusCode))
		return &RouterError{
			Code:    "LOAD_FAILED",
			Message: fmt.Sprintf("failed to load model: status %d", resp.StatusCode),
		}
	}

	r.registry.MarkModelLoaded(nodeID, model)
	return nil
}

// UnloadModel requests a specific node to unload a model
func (r *InferenceRouter) UnloadModel(ctx context.Context, nodeID, model string) error {
	node := r.registry.GetNode(nodeID)
	if node == nil {
		return &RouterError{
			Code:    "NODE_NOT_FOUND",
			Message: fmt.Sprintf("node %s not found", nodeID),
		}
	}

	log.Info().
		Str("model", model).
		Str("node", nodeID).
		Msg("Unloading model from node")

	ollamaReq := map[string]interface{}{
		"model":      model,
		"keep_alive": 0, // Unload immediately
	}

	body, _ := json.Marshal(ollamaReq)
	url := fmt.Sprintf("http://%s:%d/api/generate", node.TailscaleIP, node.Port)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("node request failed: %w", err)
	}
	defer resp.Body.Close()

	// Drain response
	io.Copy(io.Discard, resp.Body)

	// Update registry regardless of response
	if node := r.registry.GetNode(nodeID); node != nil {
		if modelInfo, exists := node.Models[model]; exists {
			modelInfo.LoadState = LoadStateIdle
		}
	}

	return nil
}
