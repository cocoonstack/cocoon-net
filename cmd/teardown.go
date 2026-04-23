package cmd

import (
	"fmt"

	"github.com/projecteru2/core/log"
	"github.com/spf13/cobra"

	"github.com/cocoonstack/cocoon-net/platform"
)

func newTeardownCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "teardown",
		Short: "Remove cloud networking resources",
		RunE:  runTeardown,
	}

	cmd.Flags().StringVar(&flagStateDir, "state-dir", defaultStateDir, "state directory")
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "show what would be done without making changes")

	return cmd
}

func runTeardown(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	logger := log.WithFunc("cmd.runTeardown")

	state, err := loadPoolState(ctx)
	if err != nil {
		return err
	}

	if flagDryRun {
		fmt.Printf("[dry-run] would teardown %s networking for node %s (subnet %s)\n",
			state.Platform, state.NodeName, state.Subnet)
		return nil
	}

	plat, err := newPlatform(ctx, state.Platform)
	if err != nil {
		return fmt.Errorf("load platform %s: %w", state.Platform, err)
	}

	td := &platform.TeardownConfig{
		AliasRangeName: state.AliasRangeName,
		SubnetCIDR:     state.Subnet,
	}
	if err := plat.Teardown(ctx, td); err != nil {
		return fmt.Errorf("teardown: %w", err)
	}
	logger.Infof(ctx, "teardown complete for %s", state.Platform)

	if err := state.Delete(ctx); err != nil {
		logger.Warnf(ctx, "delete pool state: %v", err)
	}

	fmt.Printf("cocoon-net teardown complete (%s)\n", state.Platform)
	return nil
}
