// Package fsentry wires Sentry into the gateway and worker binaries.
//
// FlowState fork only; see FLOWSTATE-FORK.md. Upstream has no Sentry.
//
// Initialising the SDK on its own reports nothing: both binaries exit through
// log.Fatal, which calls os.Exit and skips every deferred Flush, and nothing
// in pkg/ calls sentry.Capture*. So Init also installs a zerolog hook that
// reports fatal- and panic-level log events and flushes before the process
// exits, and returns a deferred func that reports a panic on main's goroutine
// and then re-raises it, so the process still crashes and restarts.
package fsentry

import (
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const flushTimeout = 2 * time.Second

// Init starts Sentry when SENTRY_DSN is set. Call it as the first line of
// main with `defer fsentry.Init()()`. With SENTRY_DSN unset it does nothing.
func Init() func() {
	dsn := os.Getenv("SENTRY_DSN")
	if dsn == "" {
		return func() {}
	}
	return start(sentry.ClientOptions{Dsn: dsn})
}

func start(opts sentry.ClientOptions) func() {
	if err := sentry.Init(opts); err != nil {
		log.Error().Err(err).Msg("sentry.Init failed")
		return func() {}
	}

	// log.Logger.Output copies hooks, so a later PrettyLogs switch keeps this.
	log.Logger = log.Logger.Hook(fatalHook{})

	return func() {
		if r := recover(); r != nil {
			sentry.CurrentHub().Recover(r)
			sentry.Flush(flushTimeout)
			panic(r)
		}
		sentry.Flush(flushTimeout)
	}
}

// fatalHook reports only fatal and panic levels. Error level is left out on
// purpose: the gateway logs routine per-request errors at that level, and
// sending them all would spend the Sentry quota on noise.
//
// A zerolog hook sees the message but not the event's fields, so the report
// carries the log message (for example "gateway stopped unexpectedly") and
// not the attached error. The error is still in the pod log.
type fatalHook struct{}

func (fatalHook) Run(_ *zerolog.Event, level zerolog.Level, msg string) {
	if level != zerolog.FatalLevel && level != zerolog.PanicLevel {
		return
	}
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentry.LevelFatal)
		sentry.CaptureMessage(msg)
	})
	sentry.Flush(flushTimeout)
}
