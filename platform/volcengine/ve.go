package volcengine

import (
	"context"
	"time"

	"github.com/cocoonstack/cocoon-net/platform"
)

// veRun is the single `ve` CLI call site; the subprocess tech debt is documented at package level.
func veRun(ctx context.Context, args ...string) ([]byte, error) {
	return platform.RunSubprocess(ctx, "ve", args...)
}

// sleepCtx returns early on cancellation so shutdown is not blocked by the ENI propagation waits.
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
