package volcengine

import (
	"context"

	"github.com/cocoonstack/cocoon-net/platform"
)

// runVe is the sole ve CLI call site; the subprocess tech debt is in the package doc.
func runVe(ctx context.Context, args ...string) ([]byte, error) {
	return platform.RunSubprocess(ctx, "ve", args...)
}
