package main

import (
	"os"
	"time"

	"github.com/beam-cloud/beta9/pkg/common"
	"github.com/beam-cloud/beta9/pkg/gateway"
	"github.com/beam-cloud/beta9/pkg/metrics"
	"github.com/beam-cloud/beta9/pkg/types"
	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	if dsn := os.Getenv("SENTRY_DSN"); dsn != "" {
		if err := sentry.Init(sentry.ClientOptions{Dsn: dsn}); err != nil {
			log.Error().Err(err).Msg("sentry.Init failed")
		} else {
			defer sentry.Flush(2 * time.Second)
		}
	}

	// Initialize logging
	configManager, err := common.NewConfigManager[types.AppConfig]()
	if err != nil {
		log.Fatal().Err(err).Msg("error creating config manager")
	}
	config := configManager.GetConfig()
	if config.DebugMode {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	if config.PrettyLogs {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	}
	metrics.InitializeMetricsRepository(config.Monitoring.VictoriaMetrics)

	gw, err := gateway.NewGateway()
	if err != nil {
		log.Fatal().Err(err).Msg("error creating gateway service")
	}

	if err := gw.Start(); err != nil {
		log.Fatal().Err(err).Msg("gateway stopped unexpectedly")
	}
	log.Info().Msg("Gateway stopped")
}
