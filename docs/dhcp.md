# Embedded DHCP Server

`cocoon-net daemon` runs its own DHCPv4 server (`dhcp/`, built on
`insomniacslk/dhcp`) on the `cni0` bridge instead of depending on an
external DHCP server. VMs plug into `cni0` via the CNI `bridge` plugin (see
[Architecture](architecture.md#cni-integration)) and obtain their
VPC-routable IP directly from this server.

## Lease lifecycle

- The pool of allocatable IPs comes from `pool.json` (the addresses the
  platform provisioner assigned at `init`/`adopt` time). GKE `init` and `adopt`
  derive up to `--pool-size` host addresses from the subnet, skipping the
  gateway and the broadcast address (so a `/24` with the default pool-size of
  140 yields 140 addresses, not all 253 usable host addresses); Volcengine
  `init` and `adopt` use the secondary IPs its ENIs report.
- Default lease time is 24 hours; expired leases are swept every minute.
- A DHCPOFFER reserves an IP for 60 seconds; if no matching DHCPREQUEST
  arrives in that window, the IP is returned to the free pool.
- Local lifecycle managers can explicitly reclaim a destroyed VM's lease with
  `DELETE /v1/leases/{mac}` over the root-only `--control-socket`. This avoids
  holding addresses until expiry when a force-stopped guest cannot send
  DHCPRELEASE. The current consumer is
  [vk-cocoon](https://github.com/cocoonstack/vk-cocoon). The endpoint returns
  `204` after a successful or already-completed release, `400` for an invalid
  MAC, and `500` when persistence fails; callers may safely retry a `500`.
- Leases are persisted to the `--lease-file` (default
  `/var/lib/cocoon/net/leases.json`, independent of `--state-dir` so
  vk-cocoon's default reader path matches) on every allocation/release, and
  reloaded at daemon startup so a restart doesn't strand or double-assign
  leases.

### Lease file format

The file is a JSON array, rewritten atomically (temp file + rename, `0644`),
so a reader either sees the previous complete file or the new one. With no live
leases it is written as `null` rather than `[]`, so decode into a slice:

```json
[
  {
    "mac": "aa:bb:cc:dd:ee:01",
    "ip": "172.20.100.37",
    "expiry": "2026-08-11T09:30:00Z"
  }
]
```

This is a consumed contract, not an internal detail:
[vk-cocoon](https://github.com/cocoonstack/vk-cocoon) resolves a freshly
cloned VM's IP by looking its MAC up here. Match on `mac` — the array order
is map-iteration order and changes between writes.

## Dynamic host routes

On every lease event the daemon updates the host's routing table so the new
IP is immediately reachable:

- **Lease granted** -- adds a `/32` route for the VM's IP via `cni0`.
- **Lease released or expired** -- removes the `/32` route. A DHCPRELEASE
  counts only when it arrives from the leased address with `ciaddr` set to it;
  the client hardware address alone is not trusted.

This keeps the kernel routing table minimal (only currently-leased IPs are
routed) and means a VM's IP is reachable within the VPC as soon as it has a
lease, with no static route provisioning per VM.

## Traffic isolation

DHCP hands every VM a VPC-routable IP, so cocoon-net also enforces isolation
between VMs at two layers:

**Same-node VM-to-VM** and **anti-spoofing** are handled at L2 by the CNI
`bridge` plugin, baked into the generated conflist:

- `portIsolation: true` -- sets the kernel `BR_ISOLATED` flag on every VM's
  veth, so same-bridge (same-node) VMs cannot exchange any frames (unicast,
  ARP, broadcast) with each other, while still reaching the bridge gateway
  and routing out. Pure L2 -- no `br_netfilter`, no conntrack.
- `macspoofchk: true` -- an nftables (bridge family) rule pinning each
  veth's source MAC to its assigned address; blocks MAC spoofing / FDB
  hijack. Stateless.

**Cross-node VM-to-VM** and **external ranges** are blocked at L3 via
`--drop-cidr`:

- `--drop-cidr` (repeatable, `init`/`adopt`) adds `FORWARD -i cni0 -d
  <CIDR> -j DROP` at the head of the chain, persisted to `pool.json` and
  reapplied by the daemon. Cross-node VM traffic is L3-routed so it
  traverses `FORWARD` naturally -- **no `br_netfilter` needed**. Pass the
  fleet VM supernet (e.g. `--drop-cidr 172.22.0.0/16`) to block VM-to-VM
  across all nodes, plus any management ranges.
- `--drop-internal-access` only adds a `FORWARD` DROP for the node's own
  `--subnet`; since same-node VM-to-VM is L2 (off `FORWARD`) and already
  covered by `portIsolation`, this flag is largely superseded by
  `--drop-cidr`.

Return traffic and internet egress are unaffected. DROP rules are tagged
`cocoon-net-drop`, so `teardown` removes exactly them.

```bash
sudo cocoon-net init \
  --platform gke --node-name cocoon-pool \
  --subnet 172.22.0.0/24 --pool-size 140 \
  --drop-cidr 172.22.0.0/16
```

> Traffic to the node's own address (e.g. a kubelet bound on the `cni0`
> gateway IP) is delivered via `INPUT`, not `FORWARD`, so these flags do
> not cover it -- restrict those separately (host `INPUT` rule or bind off
> `cni0`).

## Running the daemon

```bash
sudo cocoon-net daemon
```

The daemon loads the pool from `pool.json`, re-applies node setup (secondary
NICs up, bridge, sysctl, iptables, CNI conflist -- pass `--skip-iptables` to
omit the iptables step), and starts the DHCP server described above. It holds
`/run/cocoon-net.pid` and refuses to start while another daemon owns it. See
[Installation](installation.md#systemd-unit) for the systemd unit.

The DHCP server binds UDP port 67 on all addresses, not just `cni0`; a host
`dnsmasq` or `dhcpd` already holding that port makes the daemon fail to start.

On `cocoon-net teardown`, the cloud resources (ENIs on Volcengine, the alias
range on GKE), the tagged `cocoon-net-drop` iptables rules, `pool.json`, and the
default lease file `/var/lib/cocoon/net/leases.json` are all removed (on GKE the boot cron job that
reapplies the guest-agent route fix is removed too); a daemon started with a
custom `--lease-file` keeps that file. The `cni0` bridge, the FORWARD ACCEPT /
NAT MASQUERADE rules, and the CNI conflist are left in place.
