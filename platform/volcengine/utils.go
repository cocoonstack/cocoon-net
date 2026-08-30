package volcengine

import (
	"context"
	"time"
)

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
