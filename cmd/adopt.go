package cmd

import (
	"cmp"
	"context"
	"fmt"
	"net"
	"net/netip"
	"slices"
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

	platformName := flagPlatform
	plat, err := newPlatform(ctx, platformName)
	if err != nil {
		return fmt.Errorf("adopt platform: %w", err)
	}
	ips, poolENIs, err := adoptPool(ctx, plat, flagSubnet, gateway, flagPoolSize)
	if err != nil {
		return fmt.Errorf("compute ip list: %w", err)
	}

	primaryNIC := cmp.Or(flagPrimaryNIC, platform.DefaultNIC(platformName))
	secondaryNICCandidates := platform.DefaultSecondaryNICs(platformName)
	secondaryNICs := node.PresentLinks(secondaryNICCandidates)
	if len(secondaryNICCandidates) > 0 && len(secondaryNICs) == 0 {
		return fmt.Errorf("no secondary NIC found; expected one of %s", strings.Join(secondaryNICCandidates, ", "))
	}
	if err := requirePoolLinks(poolENIs, node.LinkMACs(secondaryNICs)); err != nil {
		return err
	}
	eniIDs := make([]string, len(poolENIs))
	for i, eni := range poolENIs {
		eniIDs[i] = eni.ID
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
		ENIIDs:             eniIDs,
		IPs:                ips,
		DNSServers:         dnsServers,
		DropInternalAccess: flagDropInternal,
		DropCIDRs:          flagDropCIDRs,
		StateDir:           flagStateDir,
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

// adoptPool is the subnet minus the gateway on GKE; on Volcengine only the ENI secondary IPs inside subnet are VPC-routed to the node, and the contributing ENIs come back for the link check.
func adoptPool(ctx context.Context, plat platform.CloudPlatform, subnet, gateway string, poolSize int) ([]string, []platform.ENIStatus, error) {
	if plat.Name() != platform.PlatformVolcengine {
		ips, err := platform.SubnetIPs(subnet, gateway, poolSize)
		return ips, nil, err
	}
	prefix, gwAddr, bcast, err := platform.HostPrefix(subnet, gateway)
	if err != nil {
		return nil, nil, err
	}
	status, err := plat.Status(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list ENI secondary IPs: %w", err)
	}
	var ips []string
	var poolENIs []platform.ENIStatus
	for _, eni := range status.ENIs {
		contributed := false
		for _, ip := range eni.IPs {
			addr, err := netip.ParseAddr(ip)
			if err != nil || !prefix.Contains(addr) || addr == gwAddr || addr == bcast || slices.Contains(ips, ip) {
				continue
			}
			ips = append(ips, ip)
			contributed = true
		}
		if contributed {
			poolENIs = append(poolENIs, eni)
		}
	}
	if len(ips) == 0 {
		return nil, nil, fmt.Errorf("no ENI secondary IP inside %s is assigned to this node", subnet)
	}
	platform.SortIPs(ips)
	return ips, poolENIs, nil
}

func requirePoolLinks(enis []platform.ENIStatus, linkMACs map[string]string) error {
	byMAC := make(map[string]string, len(linkMACs))
	for name, mac := range linkMACs {
		byMAC[canonicalMAC(mac)] = name
	}
	for _, eni := range enis {
		if _, ok := byMAC[canonicalMAC(eni.MAC)]; !ok {
			return fmt.Errorf("ENI %s (%s) carries pool IPs but no secondary link has that address", eni.ID, eni.MAC)
		}
	}
	return nil
}

func canonicalMAC(s string) string {
	if hw, err := net.ParseMAC(s); err == nil {
		return hw.String()
	}
	return strings.ToLower(s)
}
