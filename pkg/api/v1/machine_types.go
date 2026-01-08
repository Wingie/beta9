package apiv1

import "github.com/beam-cloud/beta9/pkg/types"

// MachineStateResponse provides machine state information in API responses
type MachineStateResponse struct {
	MachineID      string              `json:"machine_id"`
	Status         types.MachineStatus `json:"status"`
	PoolName       string              `json:"pool_name"`
	LastKeepalive  string              `json:"last_keepalive,omitempty"`
	LastWorkerSeen string              `json:"last_worker_seen,omitempty"`
	TTLSeconds     int                 `json:"ttl_seconds"`
	AgentVersion   string              `json:"agent_version,omitempty"`
}

// RegisterMachineResponse is returned from POST /api/v1/machine/register
type RegisterMachineResponse struct {
	Config       interface{}           `json:"config"`
	MachineState *MachineStateResponse `json:"machine_state"`
}

// KeepaliveResponse is returned from POST /api/v1/machine/keepalive
type KeepaliveResponse struct {
	Status       string                `json:"status"`
	MachineState *MachineStateResponse `json:"machine_state,omitempty"`
}

// StructuredError provides machine-readable error details
type StructuredError struct {
	ErrorCode  string            `json:"error_code"`
	Message    string            `json:"message"`
	Suggestion string            `json:"suggestion,omitempty"`
	Details    map[string]string `json:"details,omitempty"`
}

// Common error codes for machine API
const (
	ErrCodeMachineNotFound  = "machine_not_found"
	ErrCodeMachineExpired   = "machine_expired"
	ErrCodeInvalidToken     = "invalid_token"
	ErrCodeInvalidPool      = "invalid_pool"
	ErrCodeKeepaliveFailed  = "keepalive_failed"
	ErrCodeRegistrationFail = "registration_failed"
)
