package app

import (
	"context"
	"os/signal"
	"syscall"
)

// RootContext returns the process-wide context. It is cancelled on SIGINT or
// SIGTERM so that in-flight work can stop accepting new jobs, cancel child
// processes and close SQLite within a bounded time (tech-stack §88).
//
// The caller must call stop when the process is done, releasing the signal
// handler and restoring default signal behaviour for a second interrupt.
func RootContext() (ctx context.Context, stop context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}
