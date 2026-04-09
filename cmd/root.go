package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
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

// detectPlatform auto-detects the cloud platform by probing metadata endpoints concurrently.
func detectPlatform(ctx context.Context) (platform.CloudPlatform, error) {
	type result struct {
		plat platform.CloudPlatform
	}
	ch := make(chan result, 2)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if gke.Detect(ctx) {
			ch <- result{plat: &gke.GKE{}}
		}
	}()
	go func() {
		defer wg.Done()
		if volcengine.Detect(ctx) {
			ch <- result{plat: &volcengine.Volcengine{}}
		}
	}()

	// Close channel once both probes finish.
	go func() {
		wg.Wait()
		close(ch)
	}()

	for r := range ch {
		if r.plat != nil {
			return r.plat, nil
		}
	}
	return nil, fmt.Errorf("could not detect cloud platform — set --platform explicitly (%s|%s)", platform.PlatformGKE, platform.PlatformVolcengine)
}
