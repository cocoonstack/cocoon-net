package volcengine

import (
	"context"

	"github.com/cocoonstack/cocoon-net/platform"
)

// veRun is the single `ve` CLI call site; the subprocess tech debt is documented at package level.
func veRun(ctx context.Context, args ...string) ([]byte, error) {
	return platform.RunSubprocess(ctx, "ve", args...)
}
