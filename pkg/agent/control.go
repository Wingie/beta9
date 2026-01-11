package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

const DefaultControlPort = 9999

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

// Start starts the control server
func (c *ControlServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Inference control
	mux.HandleFunc("/inference/start", c.handleInferenceStart)
	mux.HandleFunc("/inference/stop", c.handleInferenceStop)
	mux.HandleFunc("/inference/status", c.handleInferenceStatus)

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
		"machine_id":        snapshot.MachineID,
		"pool":              snapshot.PoolName,
		"status":            snapshot.Status,
		"uptime_seconds":    int(snapshot.Uptime().Seconds()),
		"inference_status":  snapshot.InferenceStatus,
		"inference_port":    snapshot.InferencePort,
		"running_jobs":      snapshot.RunningJobs,
		"total_jobs":        snapshot.TotalJobs,
		"cpu_percent":       snapshot.CPUPercent,
		"memory_percent":    snapshot.MemoryPercent,
		"gpu_count":         snapshot.GPUCount,
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
