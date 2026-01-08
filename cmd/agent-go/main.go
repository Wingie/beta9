package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/beam-cloud/beta9/pkg/agent"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var version = "0.1.0"

func main() {
	// CLI flags matching Python implementation
	token := flag.String("token", os.Getenv("BETA9_TOKEN"), "Registration token from 'beta9 machine create'")
	machineID := flag.String("machine-id", os.Getenv("BETA9_MACHINE_ID"), "Unique machine ID (8 hex chars, auto-generated if not provided)")
	poolName := flag.String("pool-name", getEnvOrDefault("BETA9_POOL_NAME", "external"), "Worker pool name")
	providerName := flag.String("provider-name", getEnvOrDefault("BETA9_PROVIDER_NAME", "generic"), "Provider name")
	gatewayHost := flag.String("gateway-host", getEnvOrDefault("BETA9_GATEWAY_HOST", "localhost"), "Gateway HTTP host")
	gatewayPort := flag.Int("gateway-port", getEnvIntOrDefault("BETA9_GATEWAY_PORT", 1994), "Gateway HTTP port")
	hostname := flag.String("hostname", os.Getenv("BETA9_HOSTNAME"), "Hostname/IP for gateway to reach this machine's k3s API")
	k3sToken := flag.String("k3s-token", os.Getenv("BETA9_K3S_TOKEN"), "k3s bearer token for API authentication")
	keepaliveInterval := flag.Int("keepalive-interval", 60, "Keepalive interval in seconds")
	debug := flag.Bool("debug", getEnvBool("BETA9_DEBUG"), "Enable debug logging")
	dryRun := flag.Bool("dry-run", false, "Don't actually register, just log what would happen")
	once := flag.Bool("once", false, "Run once (register + single keepalive) then exit")
	showVersion := flag.Bool("version", false, "Show version and exit")

	flag.Parse()

	if *showVersion {
		fmt.Printf("beta9-agent-go version %s\n", version)
		return
	}

	// Configure logging (following Beta9 gateway/worker pattern)
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	// Generate machine ID if not provided
	if *machineID == "" {
		*machineID = agent.GenerateMachineID()
		log.Info().Str("machine_id", *machineID).Msg("Generated machine ID")
	}

	// Build config
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

	// Create and run agent
	a := agent.New(config)
	if err := a.Run(); err != nil {
		log.Fatal().Err(err).Msg("Agent failed")
	}
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
