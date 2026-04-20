package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const DefaultControlPort = 9999

// ControlServer handles external commands to the agent
type ControlServer struct {
	agent  *Agent
	port   int
	server *http.Server

	// ollamaBaseURL lets tests redirect Ollama calls to an httptest server.
	// Empty string means default: http://localhost:DefaultOllamaPort
	ollamaBaseURL string
}

// ollamaURL returns the Ollama base URL honoring test overrides.
func (c *ControlServer) ollamaURL() string {
	if c.ollamaBaseURL != "" {
		return c.ollamaBaseURL
	}
	return fmt.Sprintf("http://localhost:%d", DefaultOllamaPort)
}

// NewControlServer creates a new control server
func NewControlServer(agent *Agent, port int) *ControlServer {
	if port == 0 {
		port = DefaultControlPort
	}
	return &ControlServer{
		agent: agent,
		port:  port,
	}
}

// Start starts the control server
func (c *ControlServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Inference control
	mux.HandleFunc("/inference/start", c.handleInferenceStart)
	mux.HandleFunc("/inference/stop", c.handleInferenceStop)
	mux.HandleFunc("/inference/status", c.handleInferenceStatus)
	mux.HandleFunc("/inference/pull", c.handleInferencePull)

	// Agent status
	mux.HandleFunc("/status", c.handleStatus)
	mux.HandleFunc("/health", c.handleHealth)

	c.server = &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", c.port),
		Handler: mux,
	}

	go func() {
		log.Info().Int("port", c.port).Msg("Control server starting")
		if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("Control server error")
		}
	}()

	return nil
}

// Stop stops the control server
func (c *ControlServer) Stop() {
	if c.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c.server.Shutdown(ctx)
	}
}

// handleInferenceStart starts the inference server
func (c *ControlServer) handleInferenceStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Info().Msg("Control: start-inference command received")

	if err := c.agent.StartInference(); err != nil {
		log.Error().Err(err).Msg("Failed to start inference")
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	// Fetch models after startup
	models := c.getOllamaModels()
	if c.agent.state != nil && c.agent.ollama != nil && len(models) > 0 {
		c.agent.state.UpdateInference("running", c.agent.ollama.TailscaleIP(), DefaultOllamaPort, models)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"message":  "Inference server started",
		"endpoint": fmt.Sprintf("http://%s:%d", c.agent.ollama.TailscaleIP(), DefaultOllamaPort),
	})
}

// handleInferenceStop stops the inference server
func (c *ControlServer) handleInferenceStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Info().Msg("Control: stop-inference command received")

	c.agent.StopInference()

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": "Inference server stopped",
	})
}

// isAllowedModelName enforces a narrow allowlist pattern for model names
// accepted by /inference/pull. Ollama model names are of the form
// "<namespace>/<name>:<tag>" or "<name>:<tag>" — only A-Z a-z 0-9 . _ - / :
// are permitted, length <= 128. Explicitly rejects ".." to block path
// traversal payloads from reaching the Ollama daemon.
func isAllowedModelName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	// Block path-traversal sequences outright.
	if strings.Contains(name, "..") {
		return false
	}
	for _, ch := range name {
		switch {
		case ch >= 'a' && ch <= 'z':
		case ch >= 'A' && ch <= 'Z':
		case ch >= '0' && ch <= '9':
		case ch == '.' || ch == '_' || ch == '-' || ch == '/' || ch == ':':
		default:
			return false
		}
	}
	return true
}

// handleInferencePull pulls a model and streams progress to TUI logs
func (c *ControlServer) handleInferencePull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Cap request body to prevent memory-exhaustion payloads (P0-A hardening).
	r.Body = http.MaxBytesReader(w, r.Body, 4096)

	// Parse request body
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"status": "error",
			"error":  "Invalid request body",
		})
		return
	}

	if req.Model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"status": "error",
			"error":  "Model name required",
		})
		return
	}

	// P0-A model-name allowlist: reject anything with shell metacharacters
	// or path-traversal sequences before forwarding to the Ollama daemon.
	if !isAllowedModelName(req.Model) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"status": "error",
			"error":  "Invalid model name",
		})
		return
	}

	log.Info().Str("model", req.Model).Msg("Control: pull model command received")
	c.agent.state.AddLog(fmt.Sprintf("Pulling model: %s", req.Model))

	// Call Ollama pull API with timeout
	pullReq := map[string]any{"name": req.Model}
	body, _ := json.Marshal(pullReq)

	client := &http.Client{Timeout: 30 * time.Minute} // Model pulls can take a while
	resp, err := client.Post(
		fmt.Sprintf("%s/api/pull", c.ollamaURL()),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		c.agent.state.AddLog(fmt.Sprintf("Pull failed: %s", err.Error()))
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Check Ollama's status BEFORE streaming the response body. Previously a
	// non-200 from Ollama still produced StatusOK downstream because the
	// decode loop swallowed errors and the handler unconditionally returned
	// ok (Cubic finding on pkg/agent/control.go:227).
	if resp.StatusCode != http.StatusOK {
		bodyPreview := make([]byte, 512)
		n, _ := resp.Body.Read(bodyPreview)
		errMsg := string(bodyPreview[:n])
		c.agent.state.AddLog(fmt.Sprintf("Pull failed: Ollama returned HTTP %d", resp.StatusCode))
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"status":        "error",
			"error":         fmt.Sprintf("Ollama returned HTTP %d", resp.StatusCode),
			"ollama_body":   errMsg,
			"ollama_status": resp.StatusCode,
		})
		return
	}

	// Stream progress to logs. Track decode errors so a malformed stream
	// surfaces as a 502 rather than silent success.
	decoder := json.NewDecoder(resp.Body)
	lastStatus := ""
	var decodeErr error
	sawAny := false
	for {
		var progress struct {
			Status    string `json:"status"`
			Digest    string `json:"digest"`
			Total     int64  `json:"total"`
			Completed int64  `json:"completed"`
		}
		if err := decoder.Decode(&progress); err != nil {
			// io.EOF is a clean end of stream; any other error is a decode
			// failure that should be reported upstream.
			if err.Error() != "EOF" {
				decodeErr = err
			}
			break
		}
		sawAny = true

		// Update logs with progress (avoid duplicate messages)
		status := progress.Status
		if progress.Digest != "" && progress.Total > 0 {
			pct := float64(progress.Completed) / float64(progress.Total) * 100
			status = fmt.Sprintf("%s %.0f%%", progress.Status, pct)
		}
		if status != lastStatus {
			c.agent.state.AddLog(fmt.Sprintf("Pull: %s", status))
			lastStatus = status
		}
	}

	if decodeErr != nil || !sawAny {
		reason := "no progress frames received"
		if decodeErr != nil {
			reason = decodeErr.Error()
		}
		c.agent.state.AddLog(fmt.Sprintf("Pull failed: %s", reason))
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"status": "error",
			"error":  fmt.Sprintf("failed to decode Ollama response: %s", reason),
		})
		return
	}

	c.agent.state.AddLog(fmt.Sprintf("Model %s ready", req.Model))

	// Update models list
	models := c.getOllamaModels()
	if c.agent.state != nil {
		c.agent.state.UpdateInference("running", c.agent.ollama.TailscaleIP(), DefaultOllamaPort, models)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"status":  "ok",
		"message": fmt.Sprintf("Model %s pulled successfully", req.Model),
	})
}

// handleInferenceStatus returns inference server status
func (c *ControlServer) handleInferenceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	running := c.agent.IsInferenceRunning()
	status := "stopped"
	endpoint := ""

	if running {
		status = "running"
		endpoint = fmt.Sprintf("http://%s:%d", c.agent.ollama.TailscaleIP(), DefaultOllamaPort)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   status,
		"running":  running,
		"endpoint": endpoint,
		"models":   c.getOllamaModels(),
	})
}

// handleStatus returns agent status
func (c *ControlServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snapshot := c.agent.state.GetSnapshot()

	writeJSON(w, http.StatusOK, map[string]any{
		"machine_id":       snapshot.MachineID,
		"pool":             snapshot.PoolName,
		"status":           snapshot.Status,
		"uptime_seconds":   int(snapshot.Uptime().Seconds()),
		"inference_status": snapshot.InferenceStatus,
		"inference_port":   snapshot.InferencePort,
		"running_jobs":     snapshot.RunningJobs,
		"total_jobs":       snapshot.TotalJobs,
		"cpu_percent":      snapshot.CPUPercent,
		"memory_percent":   snapshot.MemoryPercent,
		"gpu_count":        snapshot.GPUCount,
	})
}

// handleHealth returns health check
func (c *ControlServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
	})
}

// getOllamaModels fetches models from Ollama API
func (c *ControlServer) getOllamaModels() []string {
	if c.agent.ollama == nil || !c.agent.ollama.IsRunning() {
		return nil
	}

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/tags", DefaultOllamaPort))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	models := make([]string, 0, len(result.Models))
	for _, m := range result.Models {
		models = append(models, m.Name)
	}
	return models
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
