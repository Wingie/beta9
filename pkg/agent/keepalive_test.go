package agent

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestKeepaliveLoopIdempotent verifies the Cubic fix for double-Stop /
// Start-after-Stop panics. Calling Stop twice must not close an already
// closed channel; calling Start after Stop must be a no-op (no new
// goroutine, no panic).
func TestKeepaliveLoopIdempotent(t *testing.T) {
	cfg := &AgentConfig{
		MachineID:         "test-machine",
		ProviderName:      "test",
		PoolName:          "test-pool",
		KeepaliveInterval: 10 * time.Millisecond,
		DryRun:            true, // skip real HTTP
	}

	loop := NewKeepaliveLoop(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop.Start(ctx)
	time.Sleep(25 * time.Millisecond) // let the goroutine tick at least once

	// Should not panic.
	loop.Stop()
	loop.Stop()

	// Start-after-Stop must be safe — no panic, no new goroutine.
	loop.Start(ctx)

	// And a third Stop must also be safe.
	loop.Stop()
}

// TestKeepaliveLoopConcurrentStop hammers Stop from many goroutines to
// confirm the sync.Once guard holds under concurrency.
func TestKeepaliveLoopConcurrentStop(t *testing.T) {
	cfg := &AgentConfig{
		MachineID:         "test-machine",
		ProviderName:      "test",
		PoolName:          "test-pool",
		KeepaliveInterval: 10 * time.Millisecond,
		DryRun:            true,
	}
	loop := NewKeepaliveLoop(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loop.Start(ctx)
	time.Sleep(15 * time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			loop.Stop()
		}()
	}
	wg.Wait()
}

// TestKeepaliveLoopStopBeforeStart verifies Stop called before Start is
// safe: no panic, no hang.
func TestKeepaliveLoopStopBeforeStart(t *testing.T) {
	cfg := &AgentConfig{
		MachineID:         "test-machine",
		ProviderName:      "test",
		PoolName:          "test-pool",
		KeepaliveInterval: time.Second,
		DryRun:            true,
	}
	loop := NewKeepaliveLoop(cfg)

	done := make(chan struct{})
	go func() {
		loop.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop-before-Start hung")
	}
}
