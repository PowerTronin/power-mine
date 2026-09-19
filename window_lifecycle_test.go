package main

import (
	"context"
	"testing"
	"time"
)

func TestWindowCloseExitWatchdogAllowsNormalClose(t *testing.T) {
	watchdog := newWindowCloseExitWatchdog(0, nil)

	if prevent := watchdog.beforeClose(context.Background()); prevent {
		t.Fatal("expected watchdog to allow normal Wails close")
	}
}

func TestWindowCloseExitWatchdogForcesExitOnce(t *testing.T) {
	exits := make(chan int, 2)
	watchdog := newWindowCloseExitWatchdog(0, func(code int) {
		exits <- code
	})

	watchdog.beforeClose(context.Background())
	watchdog.beforeClose(context.Background())

	select {
	case code := <-exits:
		if code != 0 {
			t.Fatalf("unexpected exit code: %d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("expected forced exit")
	}

	select {
	case <-exits:
		t.Fatal("expected forced exit to be scheduled once")
	case <-time.After(25 * time.Millisecond):
	}
}
