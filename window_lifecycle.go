package main

import (
	"context"
	"sync"
	"time"
)

type windowCloseExitWatchdog struct {
	delay time.Duration
	exit  func(int)
	once  sync.Once
}

func newWindowCloseExitWatchdog(delay time.Duration, exit func(int)) *windowCloseExitWatchdog {
	return &windowCloseExitWatchdog{delay: delay, exit: exit}
}

func (w *windowCloseExitWatchdog) beforeClose(context.Context) bool {
	w.once.Do(func() {
		go func() {
			// Let Wails run its normal shutdown first, then avoid hidden orphan processes.
			<-time.After(w.delay)
			if w.exit != nil {
				w.exit(0)
			}
		}()
	})
	return false
}
