package fsentry

import (
	"io"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func setup(t *testing.T) *sentry.MockTransport {
	t.Helper()
	saved := log.Logger
	t.Cleanup(func() {
		log.Logger = saved
		_ = sentry.Init(sentry.ClientOptions{})
	})
	log.Logger = zerolog.New(io.Discard)

	transport := &sentry.MockTransport{}
	start(sentry.ClientOptions{Dsn: "https://public@example.com/1", Transport: transport})
	return transport
}

func TestInitWithoutDSNIsNoop(t *testing.T) {
	t.Setenv("SENTRY_DSN", "")
	saved := log.Logger
	Init()()
	if log.Logger.GetLevel() != saved.GetLevel() {
		t.Fatal("Init without SENTRY_DSN changed the global logger")
	}
}

func TestErrorLevelIsNotReported(t *testing.T) {
	transport := setup(t)
	log.Error().Msg("routine request error")
	if n := len(transport.Events()); n != 0 {
		t.Fatalf("error-level log sent %d events, want 0", n)
	}
}

func TestPanicLevelIsReported(t *testing.T) {
	transport := setup(t)
	func() {
		defer func() { _ = recover() }()
		log.Panic().Msg("gateway stopped unexpectedly")
	}()
	events := transport.Events()
	if len(events) != 1 {
		t.Fatalf("panic-level log sent %d events, want 1", len(events))
	}
	if events[0].Message != "gateway stopped unexpectedly" || events[0].Level != sentry.LevelFatal {
		t.Fatalf("got message %q level %q", events[0].Message, events[0].Level)
	}
}

func TestPanicHookSurvivesOutputSwitch(t *testing.T) {
	transport := setup(t)
	log.Logger = log.Output(io.Discard) // what PrettyLogs does after Init
	func() {
		defer func() { _ = recover() }()
		log.Panic().Msg("after output switch")
	}()
	if n := len(transport.Events()); n != 1 {
		t.Fatalf("sent %d events after Output(), want 1", n)
	}
}

func TestMainPanicIsReportedAndReraised(t *testing.T) {
	transport := &sentry.MockTransport{}
	saved := log.Logger
	t.Cleanup(func() {
		log.Logger = saved
		_ = sentry.Init(sentry.ClientOptions{})
	})

	var reraised any
	func() {
		defer func() { reraised = recover() }()
		defer start(sentry.ClientOptions{Dsn: "https://public@example.com/1", Transport: transport})()
		panic("boom")
	}()

	if reraised != "boom" {
		t.Fatalf("panic was swallowed or changed: %v", reraised)
	}
	if n := len(transport.Events()); n != 1 {
		t.Fatalf("main panic sent %d events, want 1", n)
	}
}
