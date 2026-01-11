package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// ============================================================================
// Types - defined locally to avoid importing pkg/types (cedana dependency)
// ============================================================================

// LoadState represents model loading status
type LoadState string

const (
	LoadStateIdle    LoadState = "idle"    // Not loaded, available to pull
	LoadStateLoading LoadState = "loading" // Currently loading weights
	LoadStateReady   LoadState = "ready"   // In GPU memory, ready to serve
	LoadStateError   LoadState = "error"   // Failed to load
)

// InferenceJob represents an inference request
type InferenceJob struct {
	ID       string           `json:"id"`
	Model    string           `json:"model"`
	Messages []ChatMessage    `json:"messages,omitempty"`
	Input    string           `json:"input,omitempty"`
	Options  InferenceOptions `json:"options,omitempty"`
}

// ChatMessage represents a chat message
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// InferenceOptions contains optional inference parameters
type InferenceOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Stream      bool    `json:"stream,omitempty"`
}

// InferenceResult is the response from inference
type InferenceResult struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Content string `json:"content"`
	Error   string `json:"error,omitempty"`
}

// ModelState tracks individual model status
type ModelState struct {
	Name      string    `json:"name"`
	LoadState LoadState `json:"load_state"`
	SizeGB    float64   `json:"size_gb"`
	LastUsed  time.Time `json:"last_used"`
	Error     string    `json:"error,omitempty"`
}

// NodeInferenceStatus is reported to gateway
type NodeInferenceStatus struct {
	NodeID        string       `json:"node_id"`
	TailscaleIP   string       `json:"tailscale_ip"`
	InferencePort int          `json:"inference_port"`
	GPUType       string       `json:"gpu_type"`
	Models        []ModelState `json:"models"`
}

// ============================================================================
// OllamaManager - manages Ollama subprocess and inference
// ============================================================================

const (
	DefaultOllamaPort         = 11434
	OllamaStartTimeout        = 30 * time.Second
	OllamaHealthCheckInterval = 2 * time.Second
)

// OllamaManager manages Ollama subprocess lifecycle and inference
type OllamaManager struct {
	process     *exec.Cmd
	port        int
	tailscaleIP string
	models      map[string]*ModelState
	client      *http.Client
	mu          sync.RWMutex
	started     bool
	external    bool // true if using external Ollama instance
}

// NewOllamaManager creates a new OllamaManager
func NewOllamaManager(tailscaleIP string, port int) *OllamaManager {
	if port == 0 {
		port = DefaultOllamaPort
	}

	return &OllamaManager{
		port:        port,
		tailscaleIP: tailscaleIP,
		models:      make(map[string]*ModelState),
		client: &http.Client{
			Timeout: 5 * time.Minute, // Long timeout for model loading
		},
	}
}

// Start starts the Ollama server or connects to existing one
func (m *OllamaManager) Start(ctx context.Context) error {
	// Check if Ollama is already running
	if m.isOllamaRunning() {
		log.Info().Int("port", m.port).Msg("Ollama already running, using external instance")
		m.external = true
		m.started = true
		return nil
	}

	// Only start Ollama on macOS (for MPS support)
	if runtime.GOOS != "darwin" {
		log.Info().Msg("Not on macOS, skipping Ollama startup (use vLLM/SGLang for Linux)")
		return nil
	}

	// Check if ollama binary exists
	ollamaPath, err := exec.LookPath("ollama")
	if err != nil {
		log.Warn().Msg("Ollama not found in PATH, inference disabled")
		return nil
	}

	log.Info().Str("path", ollamaPath).Int("port", m.port).Msg("Starting Ollama server")

	// Start Ollama serve
	m.process = exec.CommandContext(ctx, ollamaPath, "serve")
	m.process.Env = append(os.Environ(),
		fmt.Sprintf("OLLAMA_HOST=0.0.0.0:%d", m.port),
		"OLLAMA_KEEP_ALIVE=24h",
	)

	// Capture output for debugging
	m.process.Stdout = &ollamaLogger{prefix: "ollama"}
	m.process.Stderr = &ollamaLogger{prefix: "ollama"}

	if err := m.process.Start(); err != nil {
		return fmt.Errorf("failed to start Ollama: %w", err)
	}

	// Wait for Ollama to be ready
	if err := m.waitForReady(ctx); err != nil {
		m.Stop()
		return fmt.Errorf("Ollama failed to start: %w", err)
	}

	m.started = true
	log.Info().
		Int("port", m.port).
		Str("tailscale_ip", m.tailscaleIP).
		Msg("Ollama server started successfully")

	return nil
}

// waitForReady waits for Ollama to respond to health checks
func (m *OllamaManager) waitForReady(ctx context.Context) error {
	deadline := time.Now().Add(OllamaStartTimeout)
	ticker := time.NewTicker(OllamaHealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if m.isOllamaRunning() {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for Ollama to start")
			}
		}
	}
}

// isOllamaRunning checks if Ollama is responding
func (m *OllamaManager) isOllamaRunning() bool {
	resp, err := m.client.Get(fmt.Sprintf("http://localhost:%d/api/tags", m.port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Stop stops the Ollama server
func (m *OllamaManager) Stop() {
	if m.process != nil && !m.external {
		log.Info().Msg("Stopping Ollama server")
		m.process.Process.Signal(os.Interrupt)
		m.process.Wait()
	}
	m.started = false
}

// IsRunning returns whether Ollama is available
func (m *OllamaManager) IsRunning() bool {
	return m.started && m.isOllamaRunning()
}

// TailscaleIP returns the Tailscale IP
func (m *OllamaManager) TailscaleIP() string {
	return m.tailscaleIP
}

// EnsureModelLoaded ensures the model is loaded and ready
func (m *OllamaManager) EnsureModelLoaded(ctx context.Context, model string) error {
	if !m.started {
		return fmt.Errorf("Ollama not started")
	}

	m.mu.Lock()
	state, exists := m.models[model]
	if !exists {
		state = &ModelState{
			Name:      model,
			LoadState: LoadStateIdle,
		}
		m.models[model] = state
	}
	m.mu.Unlock()

	if state.LoadState == LoadStateReady {
		m.mu.Lock()
		state.LastUsed = time.Now()
		m.mu.Unlock()
		return nil
	}

	return m.loadModel(ctx, model)
}

// loadModel loads a model into Ollama
func (m *OllamaManager) loadModel(ctx context.Context, model string) error {
	m.mu.Lock()
	m.models[model].LoadState = LoadStateLoading
	m.mu.Unlock()

	log.Info().Str("model", model).Msg("Loading model into Ollama")

	// Pre-warm model with keep_alive=-1 (indefinite)
	payload := map[string]any{
		"model":      model,
		"prompt":     "",
		"keep_alive": -1,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://localhost:%d/api/generate", m.port),
		bytes.NewReader(body))
	if err != nil {
		m.setModelError(model, err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		m.setModelError(model, err)
		return err
	}
	defer resp.Body.Close()

	// Drain response body (streaming)
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("failed to load model: status %d", resp.StatusCode)
		m.setModelError(model, err)
		return err
	}

	m.mu.Lock()
	m.models[model].LoadState = LoadStateReady
	m.models[model].LastUsed = time.Now()
	m.models[model].Error = ""
	m.mu.Unlock()

	log.Info().Str("model", model).Msg("Model loaded successfully")
	return nil
}

// setModelError sets error state for a model
func (m *OllamaManager) setModelError(model string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state, exists := m.models[model]; exists {
		state.LoadState = LoadStateError
		state.Error = err.Error()
	}
}

// Infer performs inference using Ollama
func (m *OllamaManager) Infer(ctx context.Context, job *InferenceJob) (*InferenceResult, error) {
	if !m.started {
		return nil, fmt.Errorf("Ollama not started")
	}

	// Ensure model is loaded
	if err := m.EnsureModelLoaded(ctx, job.Model); err != nil {
		return nil, fmt.Errorf("failed to load model: %w", err)
	}

	// Build request
	var payload map[string]any
	endpoint := "/api/chat"

	if len(job.Messages) > 0 {
		payload = map[string]any{
			"model":    job.Model,
			"messages": job.Messages,
			"stream":   false,
		}
		if job.Options.Temperature > 0 {
			payload["options"] = map[string]any{
				"temperature": job.Options.Temperature,
			}
		}
	} else if job.Input != "" {
		endpoint = "/api/generate"
		payload = map[string]any{
			"model":  job.Model,
			"prompt": job.Input,
			"stream": false,
		}
	} else {
		return nil, fmt.Errorf("no messages or input provided")
	}

	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://localhost:%d%s", m.port, endpoint),
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("inference failed: %d - %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	content := result.Message.Content
	if content == "" {
		content = result.Response
	}

	// Update last used
	m.mu.Lock()
	if state, exists := m.models[job.Model]; exists {
		state.LastUsed = time.Now()
	}
	m.mu.Unlock()

	return &InferenceResult{
		ID:      job.ID,
		Model:   job.Model,
		Content: content,
	}, nil
}

// ListModels returns current model states
func (m *OllamaManager) ListModels() []ModelState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	models := make([]ModelState, 0, len(m.models))
	for _, state := range m.models {
		models = append(models, *state)
	}
	return models
}

// UnloadModel unloads a model from memory
func (m *OllamaManager) UnloadModel(ctx context.Context, model string) error {
	if !m.started {
		return fmt.Errorf("Ollama not started")
	}

	log.Info().Str("model", model).Msg("Unloading model")

	payload := map[string]any{
		"model":      model,
		"keep_alive": 0,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://localhost:%d/api/generate", m.port),
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	m.mu.Lock()
	if state, exists := m.models[model]; exists {
		state.LoadState = LoadStateIdle
	}
	m.mu.Unlock()

	log.Info().Str("model", model).Msg("Model unloaded")
	return nil
}

// GetStatus returns inference status for gateway
func (m *OllamaManager) GetStatus() *NodeInferenceStatus {
	gpuType := ""
	if runtime.GOOS == "darwin" {
		gpuType = "MPS"
	}

	return &NodeInferenceStatus{
		TailscaleIP:   m.tailscaleIP,
		InferencePort: m.port,
		GPUType:       gpuType,
		Models:        m.ListModels(),
	}
}

// ollamaLogger captures Ollama output
type ollamaLogger struct {
	prefix string
}

func (l *ollamaLogger) Write(p []byte) (n int, err error) {
	lines := strings.Split(strings.TrimSpace(string(p)), "\n")
	for _, line := range lines {
		if line != "" {
			log.Debug().Str("source", l.prefix).Msg(line)
		}
	}
	return len(p), nil
}
