# GKE VPC-Native Networking for Cocoon VM Nodes

This guide covers how cocoon-net provisions VPC-native networking on GKE bare-metal/GCE instances so that Windows and Linux VMs running via `cocoon` are directly routable within the GKE VPC.

## Architecture

```
GKE VPC (e.g. 10.0.0.0/8)
├── Primary subnet (GKE nodes, pods)
│   └── Secondary IP range: cocoon-pods=172.20.0.0/16
│
├── cocoonset-node-1 (GCE instance)
│   ├── ens4: primary NIC (VPC)
│   │   └── alias IP range: 172.20.100.0/24
│   └── cni0 bridge (172.20.100.1/24)
│       └── Windows/Linux VMs: 172.20.100.x (DHCP from alias range)
│
└── cocoonset-node-2 (GCE instance)
    ├── ens4: primary NIC (VPC)
    │   └── alias IP range: 172.20.101.0/24
    └── cni0 bridge (172.20.101.1/24)
        └── VMs: 172.20.101.x
```

## How It Works

1. GKE subnets support **secondary IP ranges** — named CIDR blocks that can be assigned as alias IPs to GCE instances.
2. Each cocoonset node gets a `/24` alias IP range (e.g. `172.20.100.0/24`) — all IPs in this range are **VPC-routable** to that instance.
3. cocoon-net assigns the alias IPs to its embedded DHCP server pool.
4. VMs obtain IPs via DHCP from the embedded server; those IPs are within the VPC-routed alias range.
5. Any GKE pod or node can reach VM IPs directly via L3 routing — no iptables DNAT needed.

**Key caveat**: The GCE guest agent installs a `local` route for the alias CIDR in the kernel's `local` routing table, which causes the host to respond to those IPs itself (blackholing VM traffic). cocoon-net removes this route and installs a cron job to remove it on reboot.

## Prerequisites

- GKE Standard cluster with `--enable-ip-alias`
- GCE instances with `--can-ip-forward`
- GCE instance service account with `roles/compute.networkAdmin` (or equivalent)
- `gcloud` CLI installed on the node and authenticated (application default credentials or service account key)

## Running cocoon-net init

```bash
sudo cocoon-net init \
  --node-name cocoon-pool \
  --subnet 172.20.100.0/24 \
  --pool-size 140 \
  --dns 8.8.8.8,1.1.1.1
```

This will:
1. Detect the GKE platform via GCE metadata
2. Add the secondary range `cocoon-pods=172.20.100.0/24` to the node's subnet
3. Assign the alias IP `172.20.100.0/24` to `nic0` of the instance
4. Remove the local route installed by the GCE guest agent
5. Configure `cni0` bridge, iptables, sysctl
6. Write CNI conflist to `/etc/cni/net.d/30-dnsmasq-dhcp.conflist`
7. Save pool state to `/var/lib/cocoon/net/pool.json`

After init, run `cocoon-net daemon` to start the embedded DHCP server. Host routes (/32) are added dynamically when VMs obtain leases.

## Adopting existing nodes

For GKE nodes that were already provisioned by hand (alias IP range assigned, bridge configured), use `adopt` to bring them under cocoon-net management without calling any cloud APIs:

```bash
sudo cocoon-net adopt \
  --node-name cocoon-pool \
  --subnet 172.20.100.0/24
```

This configures bridge, CNI conflist, and sysctl from cocoon-net's templates, and writes the pool state file. The existing alias IP range is preserved. By default, existing iptables rules are also preserved — pass `--manage-iptables` to let cocoon-net rewrite them.

After adopting, run `cocoon-net daemon` to start DHCP. `cocoon-net status` and future re-runs of `adopt` work normally. Cloud-side teardown (removing the alias range) must still be done manually.

## Manual Steps (for reference)

### 1. Add secondary IP range to subnet

```bash
gcloud compute networks subnets update default \
  --region=asia-southeast1 \
  --add-secondary-ranges=cocoon-pods=172.20.0.0/16
```

### 2. Assign alias IP to instance

```bash
gcloud compute instances network-interfaces update cocoonset-node-1 \
  --zone=asia-southeast1-b \
  --network-interface=nic0 \
  --aliases="cocoon-pods:172.20.100.0/24"
```

### 3. Fix GCE guest agent route hijack

The GCE guest agent installs `local 172.20.100.0/24 dev ens4 table local` which blackholes inbound VM traffic. Remove it:

```bash
ip route del local 172.20.100.0/24 dev ens4 table local
```

Install cron job to remove on reboot:

```bash
echo "@reboot root ip route del local 172.20.100.0/24 dev ens4 table local 2>/dev/null || true" \
  > /etc/cron.d/cocoon-net-fix-alias
```

### 4. Configure cni0 bridge

```bash
ip link add cni0 type bridge 2>/dev/null || true
ip addr replace 172.20.100.1/24 dev cni0
ip link set cni0 up
```

### 5. sysctl

```bash
sysctl -w net.ipv4.ip_forward=1
sysctl -w net.ipv4.conf.all.rp_filter=0
sysctl -w net.ipv4.conf.cni0.rp_filter=0
sysctl -w net.ipv4.conf.ens4.rp_filter=0
```

### 6. iptables

```bash
# Allow VM traffic out via ens4 with MASQUERADE (internet access)
iptables -t nat -A POSTROUTING -s 172.20.100.0/24 ! -o cni0 -j MASQUERADE
# Allow cni0 forwarding
iptables -A FORWARD -i cni0 -o cni0 -j ACCEPT
```

### 7. DHCP

DHCP is provided by `cocoon-net daemon` (embedded server). No external dnsmasq required. Host routes (/32) are managed dynamically on lease events.

```bash
# Start the daemon (or use systemd unit)
cocoon-net daemon
```

### 8. CNI conflist

```bash
cat > /etc/cni/net.d/30-dnsmasq-dhcp.conflist <<'EOF'
{
  "cniVersion": "1.0.0",
  "name": "dnsmasq-dhcp",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "cni0",
      "isGateway": false,
      "ipMasq": false,
      "ipam": {}
    }
  ]
}
EOF
```

## IP Plan

| Range | Assignment |
|---|---|
| Primary subnet | GKE nodes, pods |
| `172.20.0.0/16` | Secondary range "cocoon-pods" (whole range registered) |
| `172.20.100.0/24` | cocoon-pool (node-1) alias IP + VM DHCP pool |
| `172.20.101.0/24` | cocoon-pool-2 (node-2) alias IP + VM DHCP pool |
| `172.20.N.0/24` | Future node-N |

## Limits

| Resource | Limit |
|---|---|
| Alias IP ranges per NIC | 10 |
| IPs per alias /24 | 254 host IPs |
| Usable DHCP IPs (pool-size 140) | 140 |

## Firewall

Allow GKE master to reach vk-cocoon kubelet API (port 10250):

```bash
gcloud compute instances add-tags cocoonset-node-1 \
  --zone=asia-southeast1-b --tags=cocoonset-node

MASTER_CIDR=$(gcloud container clusters describe <CLUSTER> \
  --zone=<ZONE> --format='value(privateClusterConfig.masterIpv4CidrBlock)')

gcloud compute firewall-rules create allow-gke-master-to-vk \
  --allow=tcp:10250 \
  --source-ranges="${MASTER_CIDR}" \
  --target-tags=cocoonset-node
```

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| VM has IP but not reachable | GCE guest agent local route | `ip route del local <cidr> dev ens4 table local` |
| No DHCP lease | Daemon not running or pool mismatch | Check `cocoon-net daemon` logs |
| kubectl exec/logs timeout | Firewall blocks port 10250 | Add firewall rule for GKE master CIDR |
| `alias IP range overlaps` | Secondary range already assigned | Use same range name `cocoon-pods` |
