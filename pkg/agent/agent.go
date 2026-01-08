package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

// Agent represents the Beta9 agent
type Agent struct {
	config        *AgentConfig
	keepaliveLoop *KeepaliveLoop
	ctx           context.Context
	cancel        context.CancelFunc
}

// New creates a new agent instance
func New(config *AgentConfig) *Agent {
	ctx, cancel := context.WithCancel(context.Background())
	return &Agent{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Run starts the agent lifecycle
func (a *Agent) Run() error {
	log.Info().
		Str("version", AgentVersion).
		Str("machine_id", a.config.MachineID).
		Str("pool", a.config.PoolName).
		Str("gateway", a.config.GatewayURL()).
		Bool("debug", a.config.Debug).
		Bool("dry_run", a.config.DryRun).
		Msg("Beta9 Agent starting")

	// Validate config
	if err := a.config.Validate(); err != nil {
		return err
	}

	// Setup signal handlers
	a.setupSignalHandlers()

	// Step 1: Register machine
	log.Info().Msg("Registering machine with gateway...")
	result := RegisterMachine(a.ctx, a.config)
	if result.Error != nil {
		return result.Error
	}

	log.Info().Msg("Machine registered successfully")
	if result.Config != nil && len(result.Config) > 0 {
		log.Debug().Interface("config", result.Config).Msg("Gateway config received")
	}

	// Step 2: Handle --once mode
	if a.config.Once {
		log.Info().Msg("Running in --once mode, sending single keepalive...")
		success := SendSingleKeepalive(a.ctx, a.config)
		if success {
			log.Info().Msg("Single keepalive sent successfully")
		} else {
			log.Warn().Msg("Single keepalive failed (may be expected if endpoint not fully deployed)")
		}
		log.Info().Msg("Agent complete (--once mode)")
		return nil
	}

	// Step 3: Start keepalive loop
	log.Info().Msg("Starting keepalive loop...")
	a.keepaliveLoop = NewKeepaliveLoop(a.config)
	a.keepaliveLoop.Start(a.ctx)

	// Step 4: Monitor health
	return a.monitorHealth()
}

func (a *Agent) setupSignalHandlers() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
		a.Shutdown()
	}()
}

func (a *Agent) monitorHealth() error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			log.Info().Msg("Agent context cancelled")
			return nil
		case <-ticker.C:
			if !a.keepaliveLoop.IsHealthy() {
				log.Error().
					Msg("Keepalive loop unhealthy (too many consecutive failures), exiting...")
				return &ErrKeepaliveFailed{
					StatusCode: 0,
					Body:       "too many consecutive failures",
				}
			}
		}
	}
}

// Shutdown gracefully stops the agent
func (a *Agent) Shutdown() {
	log.Info().Msg("Shutting down agent...")
	if a.keepaliveLoop != nil {
		a.keepaliveLoop.Stop()
	}
	a.cancel()
	log.Info().Msg("Agent shutdown complete")
}

// GenerateMachineID creates a random 8-character hex machine ID
func GenerateMachineID() string {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return hex.EncodeToString([]byte{
			byte(time.Now().UnixNano() >> 24),
			byte(time.Now().UnixNano() >> 16),
			byte(time.Now().UnixNano() >> 8),
			byte(time.Now().UnixNano()),
		})
	}
	return hex.EncodeToString(bytes)
}
