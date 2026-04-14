package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/projecteru2/core/log"
	coretypes "github.com/projecteru2/core/types"
	"github.com/spf13/cobra"

	"github.com/cocoonstack/cocoon-net/platform"
	"github.com/cocoonstack/cocoon-net/platform/gke"
	"github.com/cocoonstack/cocoon-net/platform/volcengine"
	"github.com/cocoonstack/cocoon-net/version"
)

const logLevelEnv = "COCOON_NET_LOG_LEVEL"

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

	logLevel := os.Getenv(logLevelEnv)
	if logLevel == "" {
		logLevel = "info"
	}
	if err := log.SetupLog(ctx, &coretypes.ServerLogConfig{Level: logLevel}, ""); err != nil {
		log.WithFunc("main").Fatalf(ctx, err, "setup log: %v", err)
	}

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := NewRootCmd().ExecuteContext(ctx); err != nil {
		return 1
	}
	return 0
}

// newPlatform returns a CloudPlatform by name.
func newPlatform(name string) (platform.CloudPlatform, error) {
	switch name {
	case platform.PlatformGKE:
		return &gke.GKE{}, nil
	case platform.PlatformVolcengine:
		return &volcengine.Volcengine{}, nil
	default:
		return nil, fmt.Errorf("unknown platform: %s (valid: %s, %s)", name, platform.PlatformGKE, platform.PlatformVolcengine)
	}
}
