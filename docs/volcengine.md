# Volcengine VPC-Native Networking for Cocoon VM Nodes

Make cocoon Windows/Linux VM pods directly routable within the Volcengine (火山引擎) VPC — no overlay, no iptables DNAT, no kubectl port-forward required for inter-VPC access.

## Verified Result

```
$ kubectl get pods -o wide
NAME        READY   IP              NODE
win11-0     1/1     172.20.100.6    cocoon-pool
win11-2-0   1/1     172.20.101.97   cocoon-pool-2

$ ping 172.20.100.6    # from another host in the same VPC
64 bytes from 172.20.100.6: icmp_seq=1 ttl=127 time=0.294 ms

$ nc -w 2 172.20.100.6 3389 && echo OK
OK
```

## Architecture

```
Volcengine VPC (172.20.0.0/16)
├── Subnet: 172.20.0.0/20    (host IPs — EBM nodes, VKE nodes)
├── Subnet: 172.20.100.0/24  (cocoon-pool VM IPs)
├── Subnet: 172.20.101.0/24  (cocoon-pool-2 VM IPs)
│
├── volc-ebm1 (172.20.9.230)
│   ├── eth0: primary ENI (172.20.0.0/20)
│   ├── eth1~eth7: secondary ENIs (172.20.100.0/24) ← just UP, no IP needed
│   └── cni0 bridge (172.20.100.1/24)
│       └── Windows VMs: 172.20.100.x (DHCP from secondary IP pool)
│
└── volc-ebm2 (172.20.9.231)
    ├── eth0: primary ENI (172.20.0.0/20)
    ├── eth1~eth7: secondary ENIs (172.20.101.0/24)
    └── cni0 bridge (172.20.101.1/24)
        └── Windows VMs: 172.20.101.x (DHCP)
```

## How It Works

1. Each EBM node gets a **dedicated /24 subnet** for VM IPs (separate from the host subnet).
2. Secondary ENIs are created in the VM subnet and attached to the EBM instance.
3. Secondary private IPs are assigned to each ENI — these become the DHCP pool.
4. VPC **automatically routes** the VM subnet to the ENI (L3 routing between subnets).
5. The ENI's OS-level interface (`eth1`~`eth7`) just needs to be UP — no IP configuration required.
6. Incoming packets arrive on `ethX`, kernel routes via host routes to `cni0`, bridge delivers to VM.
7. Outgoing packets: VM → `cni0` → kernel → `ethX` → VPC → destination.

**Key insight**: Same-subnet secondary IPs do NOT work for cross-host routing (VPC fabric ARP proxy black-holes inbound). Separate subnets force L3 routing, which works correctly.

## Prerequisites

- Volcengine EBM instance with ENI limit ≥ 8 (1 primary + 7 secondary)
- `ve` CLI installed and authenticated on the node
- Volcengine credentials in `~/.volcengine/config.json` or env vars:
  - `VOLCENGINE_ACCESS_KEY_ID`
  - `VOLCENGINE_SECRET_ACCESS_KEY`
  - `VOLCENGINE_REGION`

### Credentials file format

```json
{
  "access_key_id": "AKxxxx",
  "secret_access_key": "xxxx",
  "region": "cn-hongkong"
}
```

## Running cocoon-net init

```bash
sudo cocoon-net init \
  --node-name cocoon-pool \
  --subnet 172.20.100.0/24 \
  --pool-size 140 \
  --dns 8.8.8.8,1.1.1.1
```

This will:
1. Detect the Volcengine platform via instance metadata (`http://100.96.0.96/latest/meta-data/`)
2. Create subnet `172.20.100.0/24` in the VPC (if it does not exist)
3. Create 7 secondary ENIs in the VM subnet and attach them to the instance
4. Assign 20 secondary private IPs per ENI (140 total)
5. Bring up `eth1`–`eth7` interfaces
6. Configure `cni0` bridge, iptables, sysctl
7. Write CNI conflist to `/etc/cni/net.d/30-dnsmasq-dhcp.conflist`
8. Save pool state to `/var/lib/cocoon/net/pool.json`

After init, run `cocoon-net daemon` to start the embedded DHCP server. Host routes (/32) are added dynamically when VMs obtain leases.

## Adopting existing nodes

For EBM nodes that already have secondary ENIs and IPs provisioned by hand, use `adopt` to bring them under cocoon-net management without calling any Volcengine APIs:

```bash
sudo cocoon-net adopt \
  --node-name cocoon-pool \
  --subnet 172.20.100.0/24
```

This configures bridge, CNI conflist, and sysctl from cocoon-net's templates, and writes the pool state file. The existing ENIs and secondary IPs are preserved. By default, existing iptables rules are also preserved — pass `--manage-iptables` to let cocoon-net rewrite them.

After adopting, run `cocoon-net daemon` to start DHCP. `cocoon-net status` works normally. Cloud-side teardown (detaching/deleting ENIs) must still be done manually.

## Manual Steps (for reference)

### 1. Create VM subnet (once per VPC)

```bash
ve vpc CreateSubnet \
  --VpcId <vpc-id> \
  --CidrBlock 172.20.100.0/24 \
  --ZoneId cn-hongkong-a \
  --SubnetName cocoon-pool-vms
```

### 2. Create ENIs, attach, assign secondary IPs

```bash
SUBNET=<vm-subnet-id>
SG=<security-group-id>
INSTANCE=<ebm-instance-id>

for i in $(seq 1 7); do
  ENI=$(ve vpc CreateNetworkInterface \
    --SubnetId $SUBNET \
    --SecurityGroupIds.1 $SG \
    --NetworkInterfaceName "cocoon-pool-eni-${i}" \
    | python3 -c "import sys,json; print(json.load(sys.stdin)['Result']['NetworkInterfaceId'])")

  sleep 2
  ve vpc AttachNetworkInterface \
    --NetworkInterfaceId $ENI \
    --InstanceId $INSTANCE
  sleep 4

  ve vpc AssignPrivateIpAddresses \
    --NetworkInterfaceId $ENI \
    --SecondaryPrivateIpAddressCount 20
done
```

### 3. Configure the EBM node

```bash
# Bring up secondary interfaces (no IP needed)
for iface in eth1 eth2 eth3 eth4 eth5 eth6 eth7; do
  ip link set $iface up
done

# sysctl
sysctl -w net.ipv4.ip_forward=1
sysctl -w net.ipv4.conf.all.rp_filter=0
sysctl -w net.ipv4.conf.cni0.rp_filter=0
for iface in eth0 eth1 eth2 eth3 eth4 eth5 eth6 eth7; do
  sysctl -w net.ipv4.conf.${iface}.rp_filter=0
done

# cni0 bridge
ip link add cni0 type bridge 2>/dev/null || true
ip addr replace 172.20.100.1/24 dev cni0
ip link set cni0 up

# Host routes (one per secondary IP)
while read ip; do
  ip route replace ${ip}/32 dev cni0
done < /tmp/vpc-ips.txt

# iptables FORWARD
for iface in eth1 eth2 eth3 eth4 eth5 eth6 eth7; do
  iptables -A FORWARD -i $iface -o cni0 -j ACCEPT
  iptables -A FORWARD -i cni0 -o $iface -j ACCEPT
done

# NAT for outbound
iptables -t nat -A POSTROUTING -s 172.20.100.0/24 ! -o cni0 -j MASQUERADE
```

### 4. Export secondary IP list

```bash
ve vpc DescribeNetworkInterfaces \
  --InstanceId <instance-id> \
  --PageSize 100 \
  | python3 -c "
import sys, json
d = json.load(sys.stdin)
for eni in d.get('Result', {}).get('NetworkInterfaceSets', []):
    for ip in eni.get('PrivateIpSets', {}).get('PrivateIpSet', []):
        if not ip.get('Primary') and ip['PrivateIpAddress'].startswith('172.20.100'):
            print(ip['PrivateIpAddress'])
" > /tmp/vpc-ips.txt
```

### 5. DHCP

DHCP is provided by `cocoon-net daemon` (embedded server). No external dnsmasq required. Host routes (/32) are managed dynamically on lease events.

```bash
# Start the daemon (or use systemd unit)
cocoon-net daemon
```

### 6. Security group

Allow all VPC internal traffic (required — without this, cross-host packets are silently dropped):

```bash
ve vpc AuthorizeSecurityGroupIngress \
  --SecurityGroupId <sg-id> \
  --Protocol all \
  --PortStart -1 \
  --PortEnd -1 \
  --CidrIp 172.20.0.0/16 \
  --Description "VPC internal all"
```

## IP Plan

| Range | Assignment |
|---|---|
| `172.20.0.0/20` | Host subnet (EBM nodes, VKE nodes) |
| `172.20.100.0/24` | cocoon-pool (volc-ebm1) VM IPs |
| `172.20.101.0/24` | cocoon-pool-2 (volc-ebm2) VM IPs |
| `172.20.N.0/24` | Future node-N |

## Limits

| Resource | Limit | Notes |
|---|---|---|
| ENIs per EBM instance | 8 | 1 primary + 7 secondary |
| Secondary IPs per ENI | 20 | |
| Max VM IPs per node | 140 | 7 × 20 |
| Subnet size | /24 = 254 IPs | |
| VMs per /24 subnet | ~140 | 7 ENI primary IPs + 140 secondary = 147 consumed |

## GKE vs Volcengine Comparison

| Feature | GKE | Volcengine |
|---|---|---|
| Mechanism | VPC alias IP ranges | Separate subnet + secondary ENI IPs |
| Subnet per node | Not needed (alias range) | Required (/24 per node) |
| ENI configuration | N/A (single NIC) | 7 secondary ENIs, `ip link set up` |
| IP config on secondary NIC | N/A | Not needed (just UP) |
| Guest agent workaround | Yes (route hijack fix) | Not needed |
| Max IPs per node | ~254 (alias /24) | 140 (7 × 20) |
| Cross-host latency | ~0.3ms | ~0.3ms |
| Security group | Default allows VPC | Must explicitly allow VPC internal |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Cross-host ping fails, same-host works | Security group blocks VPC internal | Add `172.20.0.0/16 all` ingress rule |
| No DHCP lease | Daemon not running or pool mismatch | Check `cocoon-net daemon` logs |
| VM has IP but not reachable cross-host | `ethX` interfaces DOWN | `ip link set ethX up` |
| DHCP offers wrong subnet | Pool state has IPs from wrong subnet | Re-run `cocoon-net init` or `adopt` with correct `--subnet` |
| `InsufficientIpInSubnet` on IP assign | Orphaned ENIs consuming IPs | Delete detached ENIs in the subnet |
| Windows no DHCP, SAC stuck | Wrong cloud-hypervisor version | Use cocoon fork from cocoonstack/cloud-hypervisor |

## Adding More Nodes

For new nodes:

1. Create a new /24 subnet: `172.20.N.0/24`
2. Run on the new node:
   ```bash
   sudo cocoon-net init \
     --node-name cocoon-pool-N \
     --subnet 172.20.N.0/24 \
     --pool-size 140
   ```
3. No VPC route changes needed — subnet creation auto-adds the route.

For existing hand-provisioned nodes, use `adopt` instead of `init`:

```bash
sudo cocoon-net adopt \
  --node-name cocoon-pool-N \
  --subnet 172.20.N.0/24
```
