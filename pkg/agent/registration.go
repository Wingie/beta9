package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"
)

// RegistrationPayload is the request body for machine registration
type RegistrationPayload struct {
	Token        string `json:"token"`
	MachineID    string `json:"machine_id"`
	Hostname     string `json:"hostname"`
	ProviderName string `json:"provider_name"`
	PoolName     string `json:"pool_name"`
	CPU          string `json:"cpu"`
	Memory       string `json:"memory"`
	GPUCount     string `json:"gpu_count"`
	PrivateIP    string `json:"private_ip"`
}

// RegistrationResponse is the response from machine registration
type RegistrationResponse struct {
	Config map[string]interface{} `json:"config"`
}

// RegistrationResult contains the response from registration
type RegistrationResult struct {
	Success bool
	Config  map[string]interface{}
	Error   error
}

// RegisterMachine registers the machine with the Beta9 gateway
func RegisterMachine(ctx context.Context, config *AgentConfig) *RegistrationResult {
	hostname := config.Hostname
	if hostname == "" {
		hostname = fmt.Sprintf("machine-%s", config.MachineID)
	}

	k3sToken := config.K3sToken
	if k3sToken == "" {
		log.Warn().Msg("No --k3s-token provided. Gateway won't be able to deploy worker pods to this machine.")
	}

	gpuCount := DetectGPUCount()

	payload := &RegistrationPayload{
		Token:        k3sToken,
		MachineID:    config.MachineID,
		Hostname:     hostname,
		ProviderName: config.ProviderName,
		PoolName:     config.PoolName,
		CPU:          GetCPUString(),
		Memory:       GetMemoryString(),
		GPUCount:     strconv.Itoa(gpuCount),
		PrivateIP:    GetPrivateIP(),
	}

	log.Info().
		Str("machine_id", config.MachineID).
		Str("gateway", config.GatewayURL()).
		Str("pool", config.PoolName).
		Str("hostname", hostname).
		Int("gpu_count", gpuCount).
		Msg("Registering machine with gateway")

	log.Debug().Interface("payload", payload).Msg("Registration payload")

	if config.DryRun {
		log.Info().Msg("Dry run mode - skipping actual registration")
		return &RegistrationResult{
			Success: true,
			Config:  map[string]interface{}{"dry_run": true},
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return &RegistrationResult{Error: fmt.Errorf("failed to marshal payload: %w", err)}
	}

	client := &http.Client{Timeout: config.RegistrationTimeout}
	req, err := http.NewRequestWithContext(ctx, "POST", config.RegisterURL(), bytes.NewReader(body))
	if err != nil {
		return &RegistrationResult{Error: fmt.Errorf("failed to create request: %w", err)}
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.Token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return &RegistrationResult{
			Error: &ErrRegistrationFailed{
				Reason: fmt.Sprintf("connection failed to %s: %v (is SSH tunnel running?)", config.GatewayURL(), err),
			},
		}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		var result RegistrationResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			log.Debug().Err(err).Str("body", string(respBody)).Msg("Failed to parse registration response")
		}
		log.Info().
			Str("machine_id", config.MachineID).
			Msg("Machine registered successfully")
		return &RegistrationResult{
			Success: true,
			Config:  result.Config,
		}

	case http.StatusForbidden:
		return &RegistrationResult{
			Error: &ErrRegistrationFailed{
				StatusCode: http.StatusForbidden,
				Reason:     "Invalid token - ensure token is from 'beta9 machine create'",
			},
		}

	case http.StatusBadRequest:
		return &RegistrationResult{
			Error: &ErrRegistrationFailed{
				StatusCode: http.StatusBadRequest,
				Reason:     fmt.Sprintf("Bad request: %s", string(respBody)),
			},
		}

	default:
		return &RegistrationResult{
			Error: &ErrRegistrationFailed{
				StatusCode: resp.StatusCode,
				Reason:     fmt.Sprintf("Unexpected response: %s", string(respBody)),
			},
		}
	}
}
