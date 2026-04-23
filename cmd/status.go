package cmd

import (
	"fmt"
	"strings"

	"github.com/projecteru2/core/log"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show IP pool status",
		RunE:  runStatus,
	}

	cmd.Flags().StringVar(&flagStateDir, "state-dir", defaultStateDir, "state directory")

	return cmd
}

func runStatus(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	logger := log.WithFunc("cmd.runStatus")

	state, err := loadPoolState(ctx)
	if err != nil {
		return err
	}

	plat, err := newPlatform(ctx, state.Platform)
	if err != nil {
		return fmt.Errorf("load platform %s: %w", state.Platform, err)
	}

	status, err := plat.Status(ctx)
	if err != nil {
		logger.Warnf(ctx, "platform status unavailable: %v", err)
	}

	fmt.Printf("Platform:   %s\n", state.Platform)
	fmt.Printf("Node:       %s\n", state.NodeName)
	fmt.Printf("Subnet:     %s\n", state.Subnet)
	fmt.Printf("Gateway:    %s\n", state.Gateway)
	fmt.Printf("IPs:        %d\n", len(state.IPs))
	fmt.Printf("Updated:    %s\n", state.UpdatedAt.Format("2006-01-02T15:04:05Z"))
	if status != nil {
		if len(status.ENIIDs) > 0 || status.SubnetID != "" {
			fmt.Printf("ENIs:       %d\n", len(status.ENIIDs))
			fmt.Printf("SubnetID:   %s\n", status.SubnetID)
		}
		if len(status.AliasRanges) > 0 {
			fmt.Printf("Aliases:    %s\n", strings.Join(status.AliasRanges, ", "))
		}
	}
	return nil
}
