package volcengine

import (
	"context"

	"github.com/cocoonstack/cocoon-net/platform"
)

// runVe shells out to the ve CLI; the volcengine-go-sdk migration is pending.
func runVe(ctx context.Context, args ...string) ([]byte, error) {
	return platform.RunSubprocess(ctx, "ve", args...)
}
