package agent

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const DefaultControlPort = 9999

// DefaultControlBindAddr binds the control server to loopback only by
// default (P0-2: it used to hardcode 0.0.0.0, reachable by any Tailnet
// peer with zero authentication). Set BETA9_AGENT_CONTROL_BIND_ADDR to
// override for setups that genuinely need the gateway to reach this over
// the Tailnet — the shared-secret check below still gates every request
// either way.
const DefaultControlBindAddr = "127.0.0.1"

// isValidOllamaModelName allows only simple local model references
// (name, name:tag, namespace/name:tag) and rejects anything shaped like a
// fully-qualified third-party registry reference. P0-2b: `req.Model` used
// to be forwarded verbatim to Ollama's /api/pull, so a caller could smuggle
// "registry.attacker.com/model:tag" — a supply-chain pull plus a disk-fill
// DoS via the 30-minute pull timeout. Charset is an explicit allowlist
// (not a regex, per this repo's ban on hand-rolled regex for validation);
// the "does the first path segment contain a dot" check is the same
// heuristic Docker/Ollama use to tell a registry hostname apart from a
// plain namespace.
func isValidOllamaModelName(name string) bool {
	if name == "" || len(name) > 255 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '.' || r == '-' || r == ':' || r == '/':
		default:
			return false
		}
	}
	if strings.Contains(name, "..") || strings.Contains(name, "//") {
		return false
	}
	parts := strings.Split(name, "/")
	if len(parts) > 2 {
		return false // deeper than namespace/name is a fully-qualified external registry path
	}
	if len(parts) == 2 && strings.Contains(parts[0], ".") {
		return false // first segment looks like a registry hostname (e.g. registry.attacker.com)
	}
	return true
}

// ControlServer handles external commands to the agent
type ControlServer struct {
	agent  *Agent
	port   int
	server *http.Server
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

// requireControlToken wraps a handler so it 401s unless the caller presents
// the shared secret from BETA9_AGENT_CONTROL_TOKEN as a Bearer token or
// X-Control-Token header. P0-2: this server previously had zero
// authentication at all — any Tailnet peer reaching :9999 could start/stop
// inference or trigger a model pull. Fails closed (503) if the operator
// hasn't set a token, matching the same policy used for the gateway's
// cluster-admin auth rather than silently allowing everything.
func (c *ControlServer) requireControlToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := os.Getenv("BETA9_AGENT_CONTROL_TOKEN")
		if token == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status": "error",
				"error":  "BETA9_AGENT_CONTROL_TOKEN is not configured; refusing control requests",
			})
			return
		}
		supplied := r.Header.Get("X-Control-Token")
		if supplied == "" {
			supplied = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		if subtle.ConstantTimeCompare([]byte(supplied), []byte(token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"status": "error",
				"error":  "missing or invalid control token",
			})
			return
		}
		next(w, r)
	}
}

// Start starts the control server
func (c *ControlServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Inference control
	mux.HandleFunc("/inference/start", c.requireControlToken(c.handleInferenceStart))
	mux.HandleFunc("/inference/stop", c.requireControlToken(c.handleInferenceStop))
	mux.HandleFunc("/inference/status", c.requireControlToken(c.handleInferenceStatus))
	mux.HandleFunc("/inference/pull", c.requireControlToken(c.handleInferencePull))

	// Agent status
	mux.HandleFunc("/status", c.requireControlToken(c.handleStatus))
	mux.HandleFunc("/health", c.handleHealth) // unauthenticated liveness probe only

	bindAddr := os.Getenv("BETA9_AGENT_CONTROL_BIND_ADDR")
	if bindAddr == "" {
		bindAddr = DefaultControlBindAddr
	}
	c.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", bindAddr, c.port),
		Handler: mux,
	}

	go func() {
		log.Info().Str("bind_addr", bindAddr).Int("port", c.port).Msg("Control server starting")
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

// handleInferencePull pulls a model and streams progress to TUI logs
func (c *ControlServer) handleInferencePull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

	if !isValidOllamaModelName(req.Model) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"status": "error",
			"error":  "Model name must be a simple local reference (name[:tag] or namespace/name[:tag]), not a fully-qualified registry path",
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
		fmt.Sprintf("http://localhost:%d/api/pull", DefaultOllamaPort),
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

	// Stream progress to logs
	decoder := json.NewDecoder(resp.Body)
	lastStatus := ""
	for {
		var progress struct {
			Status    string `json:"status"`
			Digest    string `json:"digest"`
			Total     int64  `json:"total"`
			Completed int64  `json:"completed"`
		}
		if err := decoder.Decode(&progress); err != nil {
			break
		}

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

	c.agent.state.AddLog(fmt.Sprintf("Model %s ready", req.Model))

	// Update models list
	models := c.getOllamaModels()
	if c.agent.state != nil {
		c.agent.state.UpdateInference("running", c.agent.ollama.TailscaleIP(), DefaultOllamaPort, models)
	}

	writeJSON(w, http.StatusOK, map[string]any{
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
