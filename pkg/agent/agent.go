package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

const AgentVersion = "0.2.0"

// Agent represents the Beta9 agent
type Agent struct {
	config        *AgentConfig
	keepaliveLoop *KeepaliveLoop
	jobMonitor    *JobMonitor
	state         *AgentState
	tui           *TUI
	useTUI        bool
	ctx           context.Context
	cancel        context.CancelFunc
	ollama        *OllamaManager // Inference server manager
}

// New creates a new agent instance (legacy, no TUI)
func New(config *AgentConfig) *Agent {
	return NewWithTUI(config, false)
}

// NewWithTUI creates a new agent instance with optional TUI
func NewWithTUI(config *AgentConfig, useTUI bool) *Agent {
	ctx, cancel := context.WithCancel(context.Background())

	state := NewAgentState(
		config.MachineID,
		config.PoolName,
		config.GatewayURL(),
	)

	var tui *TUI
	if useTUI {
		tui = NewTUI()
	}

	// Initialize OllamaManager with Tailscale IP (or hostname)
	tailscaleIP := config.Hostname
	if tailscaleIP == "" {
		tailscaleIP = detectTailscaleIP()
	}
	ollama := NewOllamaManager(tailscaleIP, DefaultOllamaPort)

	return &Agent{
		config: config,
		state:  state,
		tui:    tui,
		useTUI: useTUI,
		ctx:    ctx,
		cancel: cancel,
		ollama: ollama,
	}
}

// detectTailscaleIP attempts to detect the Tailscale IP
func detectTailscaleIP() string {
	// Try environment variable first
	if ip := os.Getenv("TAILSCALE_IP"); ip != "" {
		return ip
	}

	// Try running tailscale ip command
	cmd := exec.Command("tailscale", "ip", "-4")
	output, err := cmd.Output()
	if err == nil {
		ip := strings.TrimSpace(string(output))
		if ip != "" {
			return ip
		}
	}

	return "localhost"
}

// Run starts the agent lifecycle
func (a *Agent) Run() error {
	if a.useTUI {
		return a.runWithTUI()
	}
	return a.runWithLogs()
}

// runWithTUI runs the agent with TUI dashboard
func (a *Agent) runWithTUI() error {
	// Enter full-screen mode (alternate screen buffer)
	a.tui.EnterFullScreen()

	// Validate config
	if err := a.config.Validate(); err != nil {
		return err
	}

	// Setup signal handlers
	a.setupSignalHandlers()

	// Start Ollama inference server (non-blocking, logs errors)
	if err := a.ollama.Start(a.ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to start Ollama, inference disabled")
	}

	// Step 1: Register machine
	a.state.Status = "REGISTERING"
	a.renderTUI()

	result := RegisterMachine(a.ctx, a.config)
	if result.Error != nil {
		a.state.Status = "ERROR"
		a.renderTUI()
		return result.Error
	}

	a.state.Status = "REGISTERED"
	a.renderTUI()

	// Handle --once mode
	if a.config.Once {
		success := SendSingleKeepalive(a.ctx, a.config)
		a.state.UpdateHeartbeat(success)
		a.renderTUI()
		time.Sleep(2 * time.Second)
		return nil
	}

	// Step 2: Start job monitor
	a.jobMonitor = NewJobMonitor(a.state)
	a.jobMonitor.RefreshPods(a.ctx)
	a.jobMonitor.Start(a.ctx)

	// Step 3: Start keepalive loop
	a.keepaliveLoop = NewKeepaliveLoopWithState(a.config, a.state)
	a.keepaliveLoop.Start(a.ctx)

	// Step 4: TUI refresh loop
	return a.tuiLoop()
}

// runWithLogs runs the agent with traditional log output
func (a *Agent) runWithLogs() error {
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

	// Start Ollama inference server
	if err := a.ollama.Start(a.ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to start Ollama, inference disabled")
	} else if a.ollama.IsRunning() {
		log.Info().
			Int("port", DefaultOllamaPort).
			Str("tailscale_ip", a.ollama.TailscaleIP()).
			Msg("Ollama inference server ready")
	}

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

// tuiLoop renders the TUI periodically
func (a *Agent) tuiLoop() error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return nil
		case <-ticker.C:
			a.renderTUI()

			// Check health
			if a.keepaliveLoop != nil && !a.keepaliveLoop.IsHealthy() {
				return &ErrKeepaliveFailed{
					StatusCode: 0,
					Body:       "too many consecutive failures",
				}
			}
		}
	}
}

// renderTUI renders the current state to terminal
func (a *Agent) renderTUI() {
	if a.tui == nil {
		return
	}

	// Update metrics from keepalive if available
	if a.keepaliveLoop != nil {
		metrics := a.keepaliveLoop.GetLastMetrics()
		a.state.UpdateMetrics(
			metrics.CpuUtilizationPct,
			metrics.MemoryUtilizationPct,
			metrics.FreeGpuCount,
		)
	}

	// Move cursor to home position (don't clear - just overwrite)
	a.tui.MoveCursorHome()
	output := a.tui.Render(a.state)
	os.Stdout.WriteString(output)
	// Clear any leftover lines from previous render
	a.tui.ClearToEnd()
}

func (a *Agent) setupSignalHandlers() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		if !a.useTUI {
			log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
		}
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
	if !a.useTUI {
		log.Info().Msg("Shutting down agent...")
	}

	if a.jobMonitor != nil {
		a.jobMonitor.Stop()
	}
	if a.keepaliveLoop != nil {
		a.keepaliveLoop.Stop()
	}
	if a.ollama != nil {
		a.ollama.Stop()
	}
	a.cancel()

	if a.useTUI && a.tui != nil {
		a.tui.ExitFullScreen()
	}

	if !a.useTUI {
		log.Info().Msg("Agent shutdown complete")
	}
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
