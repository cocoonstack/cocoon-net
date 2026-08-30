package cmd

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/projecteru2/core/log"
	"github.com/spf13/cobra"

	"github.com/cocoonstack/cocoon-net/node"
	"github.com/cocoonstack/cocoon-net/platform"
	"github.com/cocoonstack/cocoon-net/pool"
)

func newAdoptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adopt",
		Short: "Adopt an existing manually-provisioned node into cocoon-net state",
		Long: `Adopt configures a node whose cloud networking (alias IP range or
secondary ENIs) already exists. cocoon-net will bring the secondary NICs
up, configure the bridge, CNI conflist, sysctl, and write the pool state
file while leaving the cloud-side allocation untouched. Run 'cocoon-net
daemon' after adopt to start the embedded DHCP server.`,
		RunE: runAdopt,
	}

	registerCommonFlags(cmd, 253)
	cmd.Flags().BoolVar(&flagManageIPTables, "manage-iptables", false, "let cocoon-net write its FORWARD + NAT MASQUERADE rules (off by default for adopt: existing iptables on hand-provisioned hosts is preserved)")

	return cmd
}

func runAdopt(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	logger := log.WithFunc("cmd.runAdopt")

	if err := resolvePlatform(ctx); err != nil {
		return err
	}
	if err := resolveSubnet(); err != nil {
		return err
	}
	dnsServers := splitTrim(flagDNS, ",")

	gateway, err := platform.ResolveGateway(flagGateway, flagSubnet)
	if err != nil {
		return fmt.Errorf("compute gateway: %w", err)
	}

	ips, err := platform.SubnetIPs(flagSubnet, gateway, flagPoolSize)
	if err != nil {
		return fmt.Errorf("compute ip list: %w", err)
	}

	platformName := flagPlatform
	primaryNIC := cmp.Or(flagPrimaryNIC, platform.DefaultNIC(platformName))
	secondaryNICCandidates := platform.DefaultSecondaryNICs(platformName)
	secondaryNICs := node.PresentLinks(secondaryNICCandidates)
	if platformName == platform.PlatformVolcengine && len(secondaryNICs) == 0 {
		return fmt.Errorf("no secondary NIC found; expected one of %s", strings.Join(secondaryNICCandidates, ", "))
	}

	if flagDryRun {
		fmt.Println("[dry-run] would adopt node with config:")
		fmt.Printf("  platform:        %s\n", platformName)
		fmt.Printf("  node-name:       %s\n", flagNodeName)
		fmt.Printf("  subnet:          %s\n", flagSubnet)
		fmt.Printf("  gateway:         %s\n", gateway)
		fmt.Printf("  primary-nic:     %s\n", primaryNIC)
		fmt.Printf("  secondary-nics:  %s\n", strings.Join(secondaryNICs, ","))
		if len(ips) > 0 {
			fmt.Printf("  pool-size:       %d (first=%s, last=%s)\n", len(ips), ips[0], ips[len(ips)-1])
		} else {
			fmt.Printf("  pool-size:       0\n")
		}
		fmt.Printf("  dns:             %s\n", strings.Join(dnsServers, ","))
		fmt.Printf("  drop-internal:   %v\n", flagDropInternal)
		fmt.Printf("  drop-cidr:       %s\n", strings.Join(flagDropCIDRs, ","))
		fmt.Printf("  state-dir:       %s\n", flagStateDir)
		fmt.Printf("  manage-iptables: %v\n", flagManageIPTables)
		fmt.Println()
		fmt.Println("would write:")
		fmt.Println("  /etc/cni/net.d/30-cocoon-dhcp.conflist")
		fmt.Printf("  %s/pool.json\n", flagStateDir)
		networkPlan := "bridge cni0, sysctl"
		if len(secondaryNICs) > 0 {
			networkPlan = "secondary NICs up, bridge cni0, sysctl"
		}
		iptablesPlan := "skipped (preserve existing rules)"
		if flagManageIPTables {
			iptablesPlan = "(re)applied"
		}
		fmt.Printf("would (re)apply: %s; iptables %s\n", networkPlan, iptablesPlan)
		fmt.Println("routes and DHCP managed by 'cocoon-net daemon'")
		fmt.Println("would NOT touch: cloud alias IP range / ENIs (preserved as-is)")
		return nil
	}

	state := &pool.State{
		Platform:           platformName,
		NodeName:           flagNodeName,
		Subnet:             flagSubnet,
		Gateway:            gateway,
		PrimaryNIC:         primaryNIC,
		SecondaryNICs:      secondaryNICs,
		IPs:                ips,
		DNSServers:         dnsServers,
		DropInternalAccess: flagDropInternal,
		DropCIDRs:          flagDropCIDRs,
		StateDir:           flagStateDir,
	}
	plat, err := newPlatform(ctx, platformName)
	if err != nil {
		return fmt.Errorf("adopt platform: %w", err)
	}
	if err := plat.Adopt(ctx, &platform.Config{NodeName: flagNodeName, SubnetCIDR: flagSubnet, Gateway: gateway, PrimaryNIC: primaryNIC}); err != nil {
		return fmt.Errorf("adopt platform: %w", err)
	}
	if err := node.Setup(ctx, nodeConfigFromState(state, !flagManageIPTables)); err != nil {
		return fmt.Errorf("node setup: %w", err)
	}
	logger.Info(ctx, "node networking configured (adopted, cloud side untouched)")

	if err := state.Save(ctx); err != nil {
		return fmt.Errorf("save pool state: %w", err)
	}
	logger.Infof(ctx, "pool state saved to %s/pool.json", flagStateDir)

	fmt.Printf("cocoon-net adopt complete: %d IPs registered on %s (%s, cloud preserved)\n",
		len(ips), flagSubnet, platformName)
	return nil
}
