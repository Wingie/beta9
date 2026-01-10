package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/beam-cloud/beta9/pkg/agent"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var version = "0.2.0"

func main() {
	// Check for subcommands first
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			runInit()
			return
		case "version":
			fmt.Printf("b9agent version %s\n", version)
			return
		case "config":
			runConfigShow()
			return
		}
	}

	// If config exists, load from file
	if agent.ConfigExists() {
		runWithConfigFile()
		return
	}

	// Fall back to CLI flags (backward compatible)
	runWithFlags()
}

func runInit() {
	// Parse init-specific flags
	initCmd := flag.NewFlagSet("init", flag.ExitOnError)
	gatewayHost := initCmd.String("gateway", "", "Gateway host (Tailscale IP)")
	gatewayPort := initCmd.Int("port", 1994, "Gateway port")
	token := initCmd.String("token", "", "Registration token from 'beta9 machine create'")
	machineID := initCmd.String("machine-id", "", "Machine ID (8 hex chars, auto-generated if not provided)")
	pool := initCmd.String("pool", "external", "Pool name")
	hostname := initCmd.String("hostname", "", "Hostname/IP for gateway to reach k3s API")
	k3sToken := initCmd.String("k3s-token", "", "k3s bearer token")
	nonInteractive := initCmd.Bool("y", false, "Non-interactive mode (use defaults)")

	initCmd.Parse(os.Args[2:])

	// Check if running non-interactively (token provided via flag)
	isNonInteractive := *nonInteractive || (*token != "" && *gatewayHost != "")

	if !isNonInteractive {
		fmt.Println("╔══════════════════════════════════════════════════════════════╗")
		fmt.Println("║             Beta9 Agent Setup                                ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════╝")
		fmt.Println()
	}

	reader := bufio.NewReader(os.Stdin)

	// Gateway host
	gateway := *gatewayHost
	if gateway == "" {
		gateway = detectGateway()
		if !isNonInteractive {
			fmt.Printf("Gateway host [%s]: ", gateway)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input != "" {
				gateway = input
			}
		}
	}

	// Token
	regToken := *token
	if regToken == "" && !isNonInteractive {
		fmt.Print("Registration token (from 'beta9 machine create'): ")
		input, _ := reader.ReadString('\n')
		regToken = strings.TrimSpace(input)
	}

	if regToken == "" {
		fmt.Println("Error: Token is required (use --token flag)")
		os.Exit(1)
	}

	// Machine ID
	machID := *machineID
	if machID == "" {
		machID = agent.GenerateMachineID()
		if !isNonInteractive {
			fmt.Printf("Machine ID [%s]: ", machID)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input != "" {
				machID = input
			}
		}
	}

	// Pool
	poolName := *pool
	if !isNonInteractive {
		fmt.Printf("Pool name [%s]: ", poolName)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			poolName = input
		}
	}

	// Hostname (auto-detect Tailscale IP)
	hostName := *hostname
	if hostName == "" && !isNonInteractive {
		hostName = detectTailscaleIP()
		if hostName != "" {
			fmt.Printf("Hostname (Tailscale IP) [%s]: ", hostName)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input != "" {
				hostName = input
			}
		} else {
			fmt.Print("Hostname (IP for gateway to reach k3s): ")
			input, _ := reader.ReadString('\n')
			hostName = strings.TrimSpace(input)
		}
	}

	// k3s token
	k3s := *k3sToken
	if k3s == "" && !isNonInteractive {
		// Try to get k3s token from kubectl
		k3s = getK3sToken()
		if k3s != "" {
			fmt.Printf("k3s token [auto-detected]: (hidden)\n")
		} else {
			fmt.Print("k3s token (or leave empty to generate later): ")
			input, _ := reader.ReadString('\n')
			k3s = strings.TrimSpace(input)
		}
	}

	// Create config
	cfg := &agent.ConfigFile{
		Gateway: agent.GatewayConfig{
			Host: gateway,
			Port: *gatewayPort,
		},
		Machine: agent.MachineConfig{
			ID:       machID,
			Token:    regToken,
			Hostname: hostName,
		},
		Pool: poolName,
		K3s: agent.K3sConfig{
			Token: k3s,
		},
	}

	// Save config
	if err := agent.SaveConfigFile(cfg); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Config saved to: %s\n", agent.DefaultConfigPath())
	if !isNonInteractive {
		fmt.Println()
		fmt.Println("To start the agent, run:")
		fmt.Println("  b9agent")
	}
}

func runConfigShow() {
	if !agent.ConfigExists() {
		fmt.Println("No config file found. Run 'b9agent init' first.")
		os.Exit(1)
	}

	cfg, err := agent.LoadConfigFile()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Config file: %s\n\n", agent.DefaultConfigPath())
	fmt.Printf("Gateway:\n")
	fmt.Printf("  Host: %s\n", cfg.Gateway.Host)
	fmt.Printf("  Port: %d\n", cfg.Gateway.Port)
	fmt.Printf("\nMachine:\n")
	fmt.Printf("  ID: %s\n", cfg.Machine.ID)
	fmt.Printf("  Token: %s...\n", cfg.Machine.Token[:min(20, len(cfg.Machine.Token))])
	if cfg.Machine.Hostname != "" {
		fmt.Printf("  Hostname: %s\n", cfg.Machine.Hostname)
	}
	fmt.Printf("\nPool: %s\n", cfg.Pool)
	if cfg.K3s.Token != "" {
		fmt.Printf("k3s Token: (set)\n")
	}
}

func runWithConfigFile() {
	cfg, err := agent.LoadConfigFile()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		fmt.Println("Run 'b9agent init' to create a new config.")
		os.Exit(1)
	}

	// Allow CLI flags to override config file
	debug := flag.Bool("debug", cfg.Debug, "Enable debug logging")
	useTUI := flag.Bool("tui", true, "Enable TUI dashboard")
	dryRun := flag.Bool("dry-run", false, "Don't actually register")
	once := flag.Bool("once", false, "Run once then exit")
	flag.Parse()

	// Configure logging
	configureLogging(*debug)

	agentCfg := cfg.ToAgentConfig()
	agentCfg.Debug = *debug
	agentCfg.DryRun = *dryRun
	agentCfg.Once = *once

	// Create and run agent
	a := agent.NewWithTUI(agentCfg, *useTUI)
	if err := a.Run(); err != nil {
		log.Fatal().Err(err).Msg("Agent failed")
	}
}

func runWithFlags() {
	// Legacy CLI-only mode (backward compatible)
	token := flag.String("token", os.Getenv("BETA9_TOKEN"), "Registration token")
	machineID := flag.String("machine-id", os.Getenv("BETA9_MACHINE_ID"), "Machine ID")
	poolName := flag.String("pool-name", getEnvOrDefault("BETA9_POOL_NAME", "external"), "Pool name")
	providerName := flag.String("provider-name", getEnvOrDefault("BETA9_PROVIDER_NAME", "generic"), "Provider")
	gatewayHost := flag.String("gateway-host", getEnvOrDefault("BETA9_GATEWAY_HOST", "localhost"), "Gateway host")
	gatewayPort := flag.Int("gateway-port", getEnvIntOrDefault("BETA9_GATEWAY_PORT", 1994), "Gateway port")
	hostname := flag.String("hostname", os.Getenv("BETA9_HOSTNAME"), "Hostname")
	k3sToken := flag.String("k3s-token", os.Getenv("BETA9_K3S_TOKEN"), "k3s token")
	keepaliveInterval := flag.Int("keepalive-interval", 60, "Keepalive interval")
	debug := flag.Bool("debug", getEnvBool("BETA9_DEBUG"), "Debug mode")
	dryRun := flag.Bool("dry-run", false, "Dry run")
	once := flag.Bool("once", false, "Run once")
	useTUI := flag.Bool("tui", true, "Enable TUI")
	showVersion := flag.Bool("version", false, "Show version")

	flag.Parse()

	if *showVersion {
		fmt.Printf("b9agent version %s\n", version)
		return
	}

	configureLogging(*debug)

	if *machineID == "" {
		*machineID = agent.GenerateMachineID()
		log.Info().Str("machine_id", *machineID).Msg("Generated machine ID")
	}

	config := &agent.AgentConfig{
		Token:               *token,
		MachineID:           *machineID,
		PoolName:            *poolName,
		ProviderName:        *providerName,
		GatewayHost:         *gatewayHost,
		GatewayPort:         *gatewayPort,
		GatewayScheme:       "http",
		Hostname:            *hostname,
		K3sToken:            *k3sToken,
		KeepaliveInterval:   time.Duration(*keepaliveInterval) * time.Second,
		RegistrationTimeout: 30 * time.Second,
		Debug:               *debug,
		DryRun:              *dryRun,
		Once:                *once,
	}

	a := agent.NewWithTUI(config, *useTUI)
	if err := a.Run(); err != nil {
		log.Fatal().Err(err).Msg("Agent failed")
	}
}

func configureLogging(debug bool) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

func detectGateway() string {
	// Try to read from environment
	if ip := os.Getenv("TAILSCALE_CONTROLPLANE_IP"); ip != "" {
		return ip
	}
	return "localhost"
}

func detectTailscaleIP() string {
	// Try tailscale ip command
	// This is a placeholder - actual implementation would exec tailscale
	return ""
}

func getK3sToken() string {
	// Try to get k3s token via kubectl
	// kubectl create token beta9-gateway -n default --duration=8760h
	return ""
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvIntOrDefault(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvBool(key string) bool {
	val := os.Getenv(key)
	return val == "1" || val == "true" || val == "yes"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
