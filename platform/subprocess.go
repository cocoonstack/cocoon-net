package platform

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/projecteru2/core/log"
)

const subprocessWaitDelay = 2 * time.Second

// RunSubprocess runs binary with args and returns its combined output, also on failure so callers can surface CLI diagnostics.
func RunSubprocess(ctx context.Context, binary string, args ...string) ([]byte, error) {
	logger := log.WithFunc("platform.RunSubprocess")
	logger.Debugf(ctx, "spawn external binary: %s %s", binary, strings.Join(args, " "))

	//nolint:gosec // args from internal constants and metadata
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.WaitDelay = subprocessWaitDelay
	out, err := cmd.CombinedOutput()
	if errors.Is(err, exec.ErrWaitDelay) && cmd.ProcessState.Success() {
		err = nil
	}
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", binary, strings.Join(args[:min(3, len(args))], " "), err, out)
	}
	return out, nil
}
