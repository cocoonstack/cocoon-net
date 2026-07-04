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

	if err := resolvePlatform(ctx); err != nil {
		return err
	}
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
		fmt.Printf("  drop-internal: %v\n", flagDropInternal)
		fmt.Printf("  drop-cidr:  %s\n", strings.Join(flagDropCIDRs, ","))
		fmt.Printf("  state-dir:  %s\n", flagStateDir)
		return nil
	}

	// Persist a seed state before provisioning any cloud resource, so a
	// mid-provision failure still leaves teardown something to act on
	// instead of orphaning resources with no recorded state.
	state := &pool.State{
		Platform:           flagPlatform,
		NodeName:           cfg.NodeName,
		Subnet:             cfg.SubnetCIDR,
		Gateway:            cfg.Gateway,
		DNSServers:         dnsServers,
		DropInternalAccess: flagDropInternal,
		DropCIDRs:          flagDropCIDRs,
		StateDir:           flagStateDir,
	}
	if err := state.Save(ctx); err != nil {
		return fmt.Errorf("save seed pool state: %w", err)
	}

	plat, err := newPlatform(ctx, flagPlatform)
	if err != nil {
		return fmt.Errorf("init platform: %w", err)
	}
	logger.Infof(ctx, "platform: %s", plat.Name())

	result, err := plat.ProvisionNetwork(ctx, cfg)
	if err != nil {
		return fmt.Errorf("provision network: %w", err)
	}
	logger.Infof(ctx, "provisioned %d IPs on subnet %s", len(result.IPs), result.SubnetCIDR)

	state.Subnet = result.SubnetCIDR
	state.Gateway = result.Gateway
	state.PrimaryNIC = result.PrimaryNIC
	state.SecondaryNICs = result.SecondaryNICs
	state.IPs = result.IPs
	state.AliasRangeName = result.AliasRangeName
	if err := state.Save(ctx); err != nil {
		return fmt.Errorf("save pool state: %w", err)
	}
	logger.Infof(ctx, "pool state saved to %s/pool.json", flagStateDir)

	nodeCfg := &node.Config{
		Gateway:            result.Gateway,
		SubnetCIDR:         result.SubnetCIDR,
		PrimaryNIC:         result.PrimaryNIC,
		SecondaryNICs:      result.SecondaryNICs,
		DropInternalAccess: flagDropInternal,
		DropCIDRs:          flagDropCIDRs,
	}
	if err := node.Setup(ctx, nodeCfg); err != nil {
		return fmt.Errorf("node setup: %w", err)
	}
	logger.Info(ctx, "node networking configured")

	fmt.Printf("cocoon-net init complete: %d IPs provisioned on %s (%s)\n",
		len(result.IPs), result.SubnetCIDR, result.Platform)
	return nil
}
