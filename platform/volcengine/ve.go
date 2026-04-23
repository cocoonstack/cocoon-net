package volcengine

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/projecteru2/core/log"
)

// veRun shells out to the `ve` CLI. Every invocation is a tech-debt hotspot
// documented at package level — see volcengine.go. Each call is logged at
// debug level so operators can correlate slowness with external binary
// spawns.
func veRun(ctx context.Context, args ...string) ([]byte, error) {
	logger := log.WithFunc("volcengine.veRun")
	logger.Debugf(ctx, "spawn external binary: ve %s", strings.Join(args, " "))

	//nolint:gosec // args from internal constants and metadata
	cmd := exec.CommandContext(ctx, "ve", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ve %s: %w: %s", strings.Join(args[:min(3, len(args))], " "), err, out)
	}
	return out, nil
}
