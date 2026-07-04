package platform

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/projecteru2/core/log"
)

// RunSubprocess spawns binary with args and returns its combined output.
// The output is also returned alongside a wrapped error on failure, since
// callers use it to surface the underlying CLI's diagnostics.
func RunSubprocess(ctx context.Context, binary string, args ...string) ([]byte, error) {
	logger := log.WithFunc("platform.RunSubprocess")
	logger.Debugf(ctx, "spawn external binary: %s %s", binary, strings.Join(args, " "))

	//nolint:gosec // args from internal constants and metadata
	cmd := exec.CommandContext(ctx, binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", binary, strings.Join(args[:min(3, len(args))], " "), err, out)
	}
	return out, nil
}
