package node

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/projecteru2/core/log"
)

// setupRoutes adds /32 host routes for each VM IP pointing to cni0 using ip -batch.
func setupRoutes(ctx context.Context, ips []string) error {
	logger := log.WithFunc("node.setupRoutes")

	var batch strings.Builder
	for _, ip := range ips {
		fmt.Fprintf(&batch, "route replace %s/32 dev %s\n", ip, cni0Bridge)
	}

	cmd := exec.CommandContext(ctx, "ip", "-batch", "-")
	cmd.Stdin = strings.NewReader(batch.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("batch route add: %w: %s", err, out)
	}
	logger.Infof(ctx, "added %d host routes via %s", len(ips), cni0Bridge)
	return nil
}
