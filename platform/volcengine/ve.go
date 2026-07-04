package volcengine

import (
	"context"

	"github.com/cocoonstack/cocoon-net/platform"
)

// veRun shells out to the `ve` CLI. Every invocation is a tech-debt hotspot
// documented at package level — see volcengine.go.
func veRun(ctx context.Context, args ...string) ([]byte, error) {
	return platform.RunSubprocess(ctx, "ve", args...)
}
