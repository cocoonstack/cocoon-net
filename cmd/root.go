package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/cocoonstack/cocoon-net/platform"
	"github.com/cocoonstack/cocoon-net/platform/gke"
	"github.com/cocoonstack/cocoon-net/platform/volcengine"
	"github.com/cocoonstack/cocoon-net/version"
)

// NewRootCmd creates and returns the root cobra command with all subcommands registered.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "cocoon-net",
		Short:   "VPC-native networking setup for cocoon VM nodes",
		Version: version.VERSION,
	}

	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newTeardownCmd())

	return rootCmd
}

// Execute runs the root command.
func Execute() {
	code := run()
	if code != 0 {
		os.Exit(code)
	}
}

func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := NewRootCmd().ExecuteContext(ctx); err != nil {
		return 1
	}
	return 0
}

// newPlatform returns a CloudPlatform by name.
func newPlatform(name string) (platform.CloudPlatform, error) {
	switch name {
	case "gke":
		return &gke.GKE{}, nil
	case "volcengine":
		return &volcengine.Volcengine{}, nil
	default:
		return nil, fmt.Errorf("unknown platform: %s (valid: gke, volcengine)", name)
	}
}

// detectPlatform auto-detects the cloud platform by probing metadata endpoints.
func detectPlatform(ctx context.Context) (platform.CloudPlatform, error) {
	if gke.Detect(ctx) {
		return &gke.GKE{}, nil
	}
	if volcengine.Detect(ctx) {
		return &volcengine.Volcengine{}, nil
	}
	return nil, fmt.Errorf("could not detect cloud platform — set --platform explicitly (gke|volcengine)")
}
