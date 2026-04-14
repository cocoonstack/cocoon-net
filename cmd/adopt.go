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

// flagManageIPTables is the inverse of node.Config.SkipIPTables exposed only
// on the adopt subcommand: by default adopt preserves the host's existing
// firewall rules, and the operator must opt in with --manage-iptables to
// have cocoon-net rewrite them.
var flagManageIPTables bool

func newAdoptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adopt",
		Short: "Adopt an existing manually-provisioned node into cocoon-net state",
		Long: `Adopt configures a node whose cloud networking (alias IP range or
secondary ENIs) already exists. cocoon-net will overwrite the dnsmasq
config, CNI conflist, bridge, iptables, sysctl, and pool state file from
its own templates while leaving the cloud-side allocation untouched.

Use this on hosts that were provisioned by hand (or by an older script)
before cocoon-net existed, to bring them under cocoon-net management
without calling any cloud APIs.

Required flags:
  --node-name   the virtual node name (e.g. cocoon-pool, cocoon-pool-2)
  --subnet      the existing alias range CIDR (e.g. 172.20.0.0/24)`,
		RunE: runAdopt,
	}

	registerCommonFlags(cmd, 253)
	cmd.Flags().BoolVar(&flagManageIPTables, "manage-iptables", false, "let cocoon-net write its FORWARD + NAT MASQUERADE rules (off by default for adopt: existing iptables on hand-provisioned hosts is preserved)")

	return cmd
}

func runAdopt(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	logger := log.WithFunc("cmd.runAdopt")

	dnsServers := splitTrim(flagDNS, ",")

	gateway := flagGateway
	if gateway == "" {
		gw, err := platform.FirstHostIP(flagSubnet)
		if err != nil {
			return fmt.Errorf("compute default gateway from %s: %w", flagSubnet, err)
		}
		gateway = gw
	}

	ips, err := platform.SubnetIPs(flagSubnet, gateway, flagPoolSize)
	if err != nil {
		return fmt.Errorf("compute ip list: %w", err)
	}

	// Auto-detect platform name purely for the state file.
	// Default to GKE when neither metadata server answers.
	platformName := flagPlatform
	if platformName == "" {
		if plat, derr := detectPlatform(ctx); derr == nil {
			platformName = plat.Name()
		} else {
			logger.Warnf(ctx, "platform auto-detect failed (%v), defaulting to %s", derr, platform.PlatformGKE)
			platformName = platform.PlatformGKE
		}
	}

	primaryNIC := flagPrimaryNIC
	if primaryNIC == "" {
		primaryNIC = platform.DefaultNIC(platformName)
	}

	if flagDryRun {
		fmt.Println("[dry-run] would adopt node with config:")
		fmt.Printf("  platform:        %s\n", platformName)
		fmt.Printf("  node-name:       %s\n", flagNodeName)
		fmt.Printf("  subnet:          %s\n", flagSubnet)
		fmt.Printf("  gateway:         %s\n", gateway)
		fmt.Printf("  primary-nic:     %s\n", primaryNIC)
		if len(ips) > 0 {
			fmt.Printf("  pool-size:       %d (first=%s, last=%s)\n", len(ips), ips[0], ips[len(ips)-1])
		} else {
			fmt.Printf("  pool-size:       0\n")
		}
		fmt.Printf("  dns:             %s\n", strings.Join(dnsServers, ","))
		fmt.Printf("  state-dir:       %s\n", flagStateDir)
		fmt.Printf("  manage-iptables: %v\n", flagManageIPTables)
		fmt.Println()
		fmt.Println("would write:")
		fmt.Println("  /etc/cni/net.d/30-dnsmasq-dhcp.conflist")
		fmt.Printf("  %s/pool.json\n", flagStateDir)
		iptablesPlan := "skipped (preserve existing rules)"
		if flagManageIPTables {
			iptablesPlan = "(re)applied"
		}
		fmt.Printf("would (re)apply: bridge cni0, sysctl; iptables %s\n", iptablesPlan)
		fmt.Println("routes and DHCP managed by 'cocoon-net daemon'")
		fmt.Println("would NOT touch: cloud alias IP range / ENIs (preserved as-is)")
		return nil
	}

	nodeCfg := &node.Config{
		Gateway:      gateway,
		SubnetCIDR:   flagSubnet,
		PrimaryNIC:   primaryNIC,
		SkipIPTables: !flagManageIPTables,
	}
	if err := node.Setup(ctx, nodeCfg); err != nil {
		return fmt.Errorf("node setup: %w", err)
	}
	logger.Info(ctx, "node networking configured (adopted, cloud side untouched)")

	state := &pool.State{
		Platform: platformName,
		NodeName: flagNodeName,
		Subnet:   flagSubnet,
		Gateway:  gateway,
		IPs:      ips,
		StateDir: flagStateDir,
	}
	if err := state.Save(ctx); err != nil {
		return fmt.Errorf("save pool state: %w", err)
	}
	logger.Infof(ctx, "pool state saved to %s/pool.json", flagStateDir)

	fmt.Printf("cocoon-net adopt complete: %d IPs registered on %s (%s, cloud preserved)\n",
		len(ips), flagSubnet, platformName)
	return nil
}
