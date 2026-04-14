package cmd

import (
	"fmt"
	"strings"

	"github.com/projecteru2/core/log"
	"github.com/spf13/cobra"

	"github.com/cocoonstack/cocoon-net/node"
	"github.com/cocoonstack/cocoon-net/platform"
	"github.com/cocoonstack/cocoon-net/pool"
)

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Provision cloud networking and configure the node",
		RunE:  runInit,
	}

	registerCommonFlags(cmd, 140)

	return cmd
}

func runInit(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	logger := log.WithFunc("cmd.runInit")

	dnsServers := splitTrim(flagDNS, ",")

	cfg := &platform.Config{
		NodeName:   flagNodeName,
		SubnetCIDR: flagSubnet,
		PoolSize:   flagPoolSize,
		Gateway:    flagGateway,
		DNSServers: dnsServers,
		PrimaryNIC: flagPrimaryNIC,
	}

	if flagDryRun {
		fmt.Println("[dry-run] would provision networking with config:")
		fmt.Printf("  platform:   %s\n", flagPlatform)
		fmt.Printf("  node-name:  %s\n", cfg.NodeName)
		fmt.Printf("  subnet:     %s\n", cfg.SubnetCIDR)
		fmt.Printf("  pool-size:  %d\n", cfg.PoolSize)
		fmt.Printf("  dns:        %s\n", strings.Join(cfg.DNSServers, ","))
		fmt.Printf("  state-dir:  %s\n", flagStateDir)
		return nil
	}

	plat, err := newPlatform(flagPlatform)
	if err != nil {
		return fmt.Errorf("init platform: %w", err)
	}
	logger.Infof(ctx, "platform: %s", plat.Name())

	result, err := plat.ProvisionNetwork(ctx, cfg)
	if err != nil {
		return fmt.Errorf("provision network: %w", err)
	}
	logger.Infof(ctx, "provisioned %d IPs on subnet %s", len(result.IPs), result.SubnetCIDR)

	nodeCfg := &node.Config{
		Gateway:       result.Gateway,
		SubnetCIDR:    result.SubnetCIDR,
		PrimaryNIC:    result.PrimaryNIC,
		SecondaryNICs: result.SecondaryNICs,
	}
	if err := node.Setup(ctx, nodeCfg); err != nil {
		return fmt.Errorf("node setup: %w", err)
	}
	logger.Info(ctx, "node networking configured")

	state := &pool.State{
		Platform:      result.Platform,
		NodeName:      cfg.NodeName,
		Subnet:        result.SubnetCIDR,
		Gateway:       result.Gateway,
		PrimaryNIC:    result.PrimaryNIC,
		SecondaryNICs: result.SecondaryNICs,
		IPs:           result.IPs,
		StateDir:      flagStateDir,
	}
	if err := state.Save(ctx); err != nil {
		return fmt.Errorf("save pool state: %w", err)
	}
	logger.Infof(ctx, "pool state saved to %s/pool.json", flagStateDir)

	fmt.Printf("cocoon-net init complete: %d IPs provisioned on %s (%s)\n",
		len(result.IPs), result.SubnetCIDR, result.Platform)
	return nil
}
