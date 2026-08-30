package platform

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/projecteru2/core/log"
)

// RunSubprocess runs binary with args and returns its combined output, also on failure so callers can surface CLI diagnostics.
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
