package volcengine

import (
	"context"
	"time"

	"github.com/cocoonstack/cocoon-net/platform"
)

// veRun shells out to the `ve` CLI. Every invocation is a tech-debt hotspot
// documented at package level — see volcengine.go.
func veRun(ctx context.Context, args ...string) ([]byte, error) {
	return platform.RunSubprocess(ctx, "ve", args...)
}

// sleepCtx waits for d or ctx cancellation, so a SIGTERM cuts the ENI
// propagation waits short instead of blocking shutdown for seconds each.
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
