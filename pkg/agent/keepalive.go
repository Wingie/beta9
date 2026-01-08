package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

const AgentVersion = "0.1.0-go"

// KeepalivePayload is the request body for keepalive updates
type KeepalivePayload struct {
	MachineID    string          `json:"machine_id"`
	ProviderName string          `json:"provider_name"`
	PoolName     string          `json:"pool_name"`
	AgentVersion string          `json:"agent_version"`
	Metrics      *MachineMetrics `json:"metrics"`
}

// KeepaliveLoop manages periodic keepalive updates to the gateway
type KeepaliveLoop struct {
	config              *AgentConfig
	metricsCollector    *MetricsCollector
	consecutiveFailures int32
	maxFailures         int32
	stopCh              chan struct{}
	doneCh              chan struct{}
}

// NewKeepaliveLoop creates a new keepalive loop
func NewKeepaliveLoop(config *AgentConfig) *KeepaliveLoop {
	return &KeepaliveLoop{
		config:           config,
		metricsCollector: NewMetricsCollector(),
		maxFailures:      3,
		stopCh:           make(chan struct{}),
		doneCh:           make(chan struct{}),
	}
}

// Start begins the keepalive loop in a goroutine
func (k *KeepaliveLoop) Start(ctx context.Context) {
	go k.run(ctx)
}

func (k *KeepaliveLoop) run(ctx context.Context) {
	defer close(k.doneCh)

	log.Info().
		Dur("interval", k.config.KeepaliveInterval).
		Msg("Started keepalive loop")

	// Send first keepalive immediately
	k.sendKeepalive(ctx)

	ticker := time.NewTicker(k.config.KeepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Keepalive loop stopped (context cancelled)")
			return
		case <-k.stopCh:
			log.Info().Msg("Keepalive loop stopped (stop signal)")
			return
		case <-ticker.C:
			k.sendKeepalive(ctx)
		}
	}
}

// Stop signals the keepalive loop to stop and waits for it to finish
func (k *KeepaliveLoop) Stop() {
	close(k.stopCh)
	// Wait for loop to finish with timeout
	select {
	case <-k.doneCh:
	case <-time.After(5 * time.Second):
		log.Warn().Msg("Keepalive loop did not stop within timeout")
	}
}

// IsHealthy returns true if recent keepalives succeeded
func (k *KeepaliveLoop) IsHealthy() bool {
	return atomic.LoadInt32(&k.consecutiveFailures) < k.maxFailures
}

func (k *KeepaliveLoop) sendKeepalive(ctx context.Context) bool {
	metrics, err := k.metricsCollector.Collect()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to collect metrics")
		metrics = &MachineMetrics{}
	}

	payload := &KeepalivePayload{
		MachineID:    k.config.MachineID,
		ProviderName: k.config.ProviderName,
		PoolName:     k.config.PoolName,
		AgentVersion: AgentVersion,
		Metrics:      metrics,
	}

	log.Debug().
		Float64("cpu_pct", metrics.CpuUtilizationPct).
		Float64("mem_pct", metrics.MemoryUtilizationPct).
		Int("free_gpu", metrics.FreeGpuCount).
		Msg("Sending keepalive")

	if k.config.DryRun {
		log.Info().Msg("Dry run - skipping keepalive")
		return true
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal keepalive payload")
		return false
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "POST", k.config.KeepaliveURL(), bytes.NewReader(body))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create keepalive request")
		return false
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", k.config.Token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		failures := atomic.AddInt32(&k.consecutiveFailures, 1)
		log.Warn().
			Err(err).
			Int32("failure_count", failures).
			Msg("Keepalive connection failed")
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		atomic.StoreInt32(&k.consecutiveFailures, 0)
		log.Debug().
			Str("machine_id", k.config.MachineID).
			Msg("Keepalive successful")
		return true
	}

	failures := atomic.AddInt32(&k.consecutiveFailures, 1)
	log.Warn().
		Int("status", resp.StatusCode).
		Int32("failure_count", failures).
		Int32("max_failures", k.maxFailures).
		Msg("Keepalive failed")
	return false
}

// SendSingleKeepalive sends a single keepalive update (for testing/once mode)
func SendSingleKeepalive(ctx context.Context, config *AgentConfig) bool {
	loop := NewKeepaliveLoop(config)
	return loop.sendKeepalive(ctx)
}
