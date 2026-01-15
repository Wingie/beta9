package types

import "time"

// ============================================================================
// Inference Types - Shared across Gateway, Agent, and API
// ============================================================================

// LoadState represents model loading status
type LoadState string

const (
	LoadStateIdle    LoadState = "idle"    // Not loaded, available to pull
	LoadStateLoading LoadState = "loading" // Currently loading weights
	LoadStateReady   LoadState = "ready"   // In GPU memory, ready to serve
	LoadStateError   LoadState = "error"   // Failed to load
)

// NodeInferenceInfo tracks inference capability of a node
// Used in ModelRegistry and API calls
type NodeInferenceInfo struct {
	NodeID        string                `json:"node_id"`
	TailscaleIP   string                `json:"tailscale_ip"`
	Port          int                   `json:"port"`
	GPUType       string                `json:"gpu_type"` // "MPS", "CUDA", "ROCm", "CPU"
	TotalVRAM     int64                 `json:"total_vram_mb"`
	AvailableVRAM int64                 `json:"available_vram_mb"`
	Models        map[string]*ModelInfo `json:"models"`
	LastHeartbeat time.Time             `json:"last_heartbeat"`
	Healthy       bool                  `json:"healthy"`
}

// ModelInfo tracks individual model status on a node
type ModelInfo struct {
	Name         string    `json:"name"`
	LoadState    LoadState `json:"load_state"`
	SizeGB       float64   `json:"size_gb"`
	LastUsed     time.Time `json:"last_used"`
	LoadedAt     time.Time `json:"loaded_at"`
	RequestCount int64     `json:"request_count"`
	Error        string    `json:"error,omitempty"`
}

// InferenceStatus is used in keepalive payloads (Agent -> Gateway)
type InferenceStatus struct {
	Status string   `json:"status"` // stopped, starting, running, error
	IP     string   `json:"ip,omitempty"`
	Port   int      `json:"port,omitempty"`
	Models []string `json:"models,omitempty"`
}
