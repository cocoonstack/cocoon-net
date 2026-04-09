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

// adopt subcommand reuses the existing cloud-side networking (alias IP range
// or secondary ENIs that some other operator already provisioned) and only
// runs the host-side configuration steps + writes the pool state file. Use
// this on nodes that were brought up by hand (or by an older provisioning
// script) before cocoon-net existed: it lets cocoon-net manage the dnsmasq
// config, CNI conflist, bridge, iptables, sysctl, and state file going
// forward without needing to talk to the cloud API at all.
//
// Compared to `init`:
//   - skips platform.ProvisionNetwork (no gcloud / volcengine API calls)
//   - the operator must supply --subnet and (optionally) --gateway and
//     --pool-size that match what is already plumbed in the cloud
//   - everything else (node.Setup + pool.Save) runs identically
//
// `cocoon-net status` will work afterwards because pool.json now exists, and
// any later `cocoon-net teardown` will delete the same files cocoon-net wrote
// here. Cloud-side teardown still needs to be done by hand on adopted nodes
// (cocoon-net will not undo what it did not provision).
func newAdoptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adopt",
		Short: "Adopt an existing manually-provisioned node into cocoon-net state",
		Long: `Adopt configures a node whose cloud networking (alias IP range or
secondary ENIs) already exists. cocoon-net will overwrite the dnsmasq
config, CNI conflist, bridge, iptables, sysctl, and pool state file from
its own templates while leaving the cloud-side allocation untouched.

Use this on the simular cluster's cocoonset-node-* hosts, which were
provisioned by hand before cocoon-net existed.

Required flags:
  --node-name   the virtual node name (e.g. cocoon-pool, cocoon-pool-2)
  --subnet      the existing alias range CIDR (e.g. 172.20.0.0/24)`,
		RunE: runAdopt,
	}

	cmd.Flags().StringVar(&flagPlatform, "platform", "", "platform name to record in pool state (auto-detect if empty)")
	cmd.Flags().StringVar(&flagNodeName, "node-name", "", "virtual node name (required)")
	cmd.Flags().StringVar(&flagSubnet, "subnet", "", "existing VM subnet CIDR (required)")
	cmd.Flags().IntVar(&flagPoolSize, "pool-size", 253, "number of IPs to write into dnsmasq + state (default fits a /24 minus gateway/network/broadcast)")
	cmd.Flags().StringVar(&flagGateway, "gateway", "", "gateway IP on cni0 (default: first host IP in --subnet)")
	cmd.Flags().StringVar(&flagPrimaryNIC, "primary-nic", "", "host primary NIC (auto-detect if empty)")
	cmd.Flags().StringVar(&flagDNS, "dns", "8.8.8.8,1.1.1.1", "comma-separated DNS servers for DHCP clients")
	cmd.Flags().StringVar(&flagStateDir, "state-dir", defaultStateDir, "state directory")
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "show what would be done without making changes")
	cmd.Flags().BoolVar(&flagManageIPTables, "manage-iptables", false, "let cocoon-net write its FORWARD + NAT MASQUERADE rules (off by default for adopt: existing iptables on hand-provisioned hosts is preserved)")

	_ = cmd.MarkFlagRequired("node-name")
	_ = cmd.MarkFlagRequired("subnet")

	return cmd
}

func runAdopt(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	logger := log.WithFunc("cmd.runAdopt")

	dnsServers := splitTrim(flagDNS, ",")

	// Default gateway = first host IP of the subnet, matching what
	// platform.FirstHostIP would return for a fresh init.
	gateway := flagGateway
	if gateway == "" {
		gw, err := platform.FirstHostIP(flagSubnet)
		if err != nil {
			return fmt.Errorf("compute default gateway from %s: %w", flagSubnet, err)
		}
		gateway = gw
	}

	// IPs come from the local subnet helper to keep parity with init's
	// `result.IPs` — same generation order, same gateway-skip behavior.
	ips, err := platform.SubnetIPs(flagSubnet, gateway, flagPoolSize)
	if err != nil {
		return fmt.Errorf("compute ip list: %w", err)
	}

	// Auto-detect platform name (purely for the state file's `platform` field).
	// Pool state is the only place this matters; node.Setup itself does not
	// branch on platform. Default to "gke" when neither metadata server
	// answers — operators on bare metal can pass --platform explicitly.
	platformName := flagPlatform
	if platformName == "" {
		if plat, derr := detectPlatform(ctx); derr == nil {
			platformName = plat.Name()
		} else {
			logger.Warnf(ctx, "platform auto-detect failed (%v), defaulting to gke", derr)
			platformName = "gke"
		}
	}

	primaryNIC := flagPrimaryNIC
	if primaryNIC == "" {
		primaryNIC = "ens4" // matches gke.defaultNIC; node.Setup also auto-detects via /sys
	}

	if flagDryRun {
		fmt.Println("[dry-run] would adopt node with config:")
		fmt.Printf("  platform:        %s\n", platformName)
		fmt.Printf("  node-name:       %s\n", flagNodeName)
		fmt.Printf("  subnet:          %s\n", flagSubnet)
		fmt.Printf("  gateway:         %s\n", gateway)
		fmt.Printf("  primary-nic:     %s\n", primaryNIC)
		fmt.Printf("  pool-size:       %d (first=%s, last=%s)\n", len(ips), ips[0], ips[len(ips)-1])
		fmt.Printf("  dns:             %s\n", strings.Join(dnsServers, ","))
		fmt.Printf("  state-dir:       %s\n", flagStateDir)
		fmt.Printf("  manage-iptables: %v\n", flagManageIPTables)
		fmt.Println()
		fmt.Println("would write:")
		fmt.Println("  /etc/cni/net.d/30-dnsmasq-dhcp.conflist")
		fmt.Println("  /etc/dnsmasq-cni.d/cni0.conf")
		fmt.Printf("  %s/pool.json\n", flagStateDir)
		iptablesPlan := "skipped (preserve existing rules)"
		if flagManageIPTables {
			iptablesPlan = "(re)applied"
		}
		fmt.Printf("would (re)apply: bridge cni0, sysctl, host routes, dnsmasq-cni restart; iptables %s\n", iptablesPlan)
		fmt.Println("would NOT touch: cloud alias IP range / ENIs (preserved as-is)")
		return nil
	}

	nodeCfg := &node.Config{
		Gateway:      gateway,
		SubnetCIDR:   flagSubnet,
		IPs:          ips,
		DNSServers:   dnsServers,
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

// splitTrim splits and trims whitespace from each element. Used by adopt and
// init both — keep close to runInit's parsing of --dns to avoid divergence.
func splitTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
