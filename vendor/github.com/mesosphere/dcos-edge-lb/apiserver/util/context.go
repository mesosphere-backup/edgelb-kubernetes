package util

import (
	"context"
	"time"
)

// CtxTimeoutCallbackFn is called at timeout
type CtxTimeoutCallbackFn func()

// CtxDoneCallbackFn is called when done
type CtxDoneCallbackFn func()

// CtxTimeout cancels the context after a set duration
func CtxTimeout(ctx context.Context, cancel context.CancelFunc,
	timeout time.Duration, timedOut CtxTimeoutCallbackFn,
	done CtxDoneCallbackFn) {

	select {
	case <-time.After(timeout):
		timedOut()
		cancel()
	case <-ctx.Done():
		done()
	}
}
