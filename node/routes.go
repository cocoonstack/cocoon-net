package node

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/projecteru2/core/log"
)

// setupRoutes adds /32 host routes for each VM IP pointing to cni0.
func setupRoutes(ctx context.Context, ips []string) error {
	logger := log.WithFunc("node.setupRoutes")

	for _, ip := range ips {
		//nolint:gosec // ip comes from cloud API, not user input
		cmd := exec.CommandContext(ctx, "ip", "route", "replace",
			ip+"/32", "dev", cni0Bridge,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("add route for %s: %w: %s", ip, err, out)
		}
	}
	logger.Infof(ctx, "added %d host routes via %s", len(ips), cni0Bridge)
	return nil
}
