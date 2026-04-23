package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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

	commonlog.Setup(ctx, "COCOON_NET_LOG_LEVEL")

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := NewRootCmd().ExecuteContext(ctx); err != nil {
		return 1
	}
	return 0
}

// newPlatform returns a CloudPlatform by name.
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
