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

var (
	flagPlatform   string
	flagNodeName   string
	flagSubnet     string
	flagPoolSize   int
	flagGateway    string
	flagPrimaryNIC string
	flagDNS        string
	flagStateDir   string
	flagDryRun     bool
)

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Provision cloud networking and configure the node",
		RunE:  runInit,
	}

	cmd.Flags().StringVar(&flagPlatform, "platform", "", "force platform (gke|volcengine); auto-detect if not set")
	cmd.Flags().StringVar(&flagNodeName, "node-name", "", "virtual node name (required)")
	cmd.Flags().StringVar(&flagSubnet, "subnet", "", "VM subnet CIDR, e.g. 172.20.100.0/24 (required)")
	cmd.Flags().IntVar(&flagPoolSize, "pool-size", 140, "number of IPs to provision")
	cmd.Flags().StringVar(&flagGateway, "gateway", "", "gateway IP on cni0 (default: first IP in subnet)")
	cmd.Flags().StringVar(&flagPrimaryNIC, "primary-nic", "", "host primary NIC (auto-detect if not set)")
	cmd.Flags().StringVar(&flagDNS, "dns", "8.8.8.8,1.1.1.1", "comma-separated DNS servers for DHCP clients")
	cmd.Flags().StringVar(&flagStateDir, "state-dir", "/var/lib/cocoon/net", "state directory")
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "show what would be done without making changes")

	_ = cmd.MarkFlagRequired("node-name")
	_ = cmd.MarkFlagRequired("subnet")

	return cmd
}

func runInit(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	logger := log.WithFunc("cmd.runInit")

	dnsServers := strings.Split(flagDNS, ",")
	for i := range dnsServers {
		dnsServers[i] = strings.TrimSpace(dnsServers[i])
	}

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

	// Detect or validate platform.
	var plat platform.CloudPlatform
	var err error
	if flagPlatform != "" {
		plat, err = platform.New(flagPlatform)
	} else {
		plat, err = platform.Detect(ctx)
	}
	if err != nil {
		return fmt.Errorf("detect platform: %w", err)
	}
	logger.Infof(ctx, "platform: %s", plat.Name())

	// Provision cloud networking.
	result, err := plat.ProvisionNetwork(ctx, cfg)
	if err != nil {
		return fmt.Errorf("provision network: %w", err)
	}
	logger.Infof(ctx, "provisioned %d IPs on subnet %s", len(result.IPs), result.SubnetCIDR)

	// Configure node networking.
	nodeCfg := &node.Config{
		Gateway:    result.Gateway,
		SubnetCIDR: result.SubnetCIDR,
		IPs:        result.IPs,
		DNSServers: cfg.DNSServers,
		PrimaryNIC: result.PrimaryNIC,
	}
	if err := node.Setup(ctx, nodeCfg); err != nil {
		return fmt.Errorf("node setup: %w", err)
	}
	logger.Info(ctx, "node networking configured")

	// Write pool state.
	state := &pool.State{
		Platform: result.Platform,
		NodeName: cfg.NodeName,
		Subnet:   result.SubnetCIDR,
		Gateway:  result.Gateway,
		IPs:      result.IPs,
		StateDir: flagStateDir,
	}
	if err := state.Save(ctx); err != nil {
		return fmt.Errorf("save pool state: %w", err)
	}
	logger.Infof(ctx, "pool state saved to %s/pool.json", flagStateDir)

	fmt.Printf("cocoon-net init complete: %d IPs provisioned on %s (%s)\n",
		len(result.IPs), result.SubnetCIDR, result.Platform)
	return nil
}
