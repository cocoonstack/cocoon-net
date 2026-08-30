// Package cmd wires cocoon-net cobra commands.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/projecteru2/core/log"
	"github.com/spf13/cobra"

	commonlog "github.com/cocoonstack/cocoon-common/log"
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
	rootCmd.AddCommand(newAdoptCmd())
	rootCmd.AddCommand(newDaemonCmd())
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
	ctx := context.Background()

	if err := commonlog.Setup(ctx, "COCOON_NET_LOG_LEVEL"); err != nil {
		fmt.Fprintf(os.Stderr, "setup log: %v\n", err)
		return 1
	}

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := NewRootCmd().ExecuteContext(ctx); err != nil {
		return 1
	}
	return 0
}

// newPlatform never auto-detects, keeping teardown/status offline-safe; empty names go through resolvePlatform first.
func newPlatform(ctx context.Context, name string) (platform.CloudPlatform, error) {
	switch name {
	case platform.PlatformGKE:
		return gke.New(), nil
	case platform.PlatformVolcengine:
		return volcengine.New(ctx)
	default:
		return nil, fmt.Errorf("unknown platform: %s (valid: %s, %s)", name, platform.PlatformGKE, platform.PlatformVolcengine)
	}
}

// detectPlatform probes GKE first: its DNS name fails in milliseconds, while the Volcengine address black-holes for the full timeout.
func detectPlatform(ctx context.Context) (string, error) {
	logger := log.WithFunc("cmd.detectPlatform")
	if gke.Detect(ctx) {
		logger.Infof(ctx, "detected platform: %s", platform.PlatformGKE)
		return platform.PlatformGKE, nil
	}
	if volcengine.Detect(ctx) {
		logger.Infof(ctx, "detected platform: %s", platform.PlatformVolcengine)
		return platform.PlatformVolcengine, nil
	}
	return "", errors.New("platform auto-detection failed: no metadata endpoint responded; pass --platform gke|volcengine")
}
