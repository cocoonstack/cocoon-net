# cocoon-net

`cocoon-net` automates VPC-native networking setup for [cocoon](https://github.com/cocoonstack/cocoon) VM nodes on GKE and Volcengine cloud platforms.

On each bare-metal/VM node, `cocoon-net` provisions cloud networking resources and configures the Linux host so that Windows and Linux VMs obtain VPC-routable IPs directly via DHCP — no overlay network, no iptables DNAT, no `kubectl port-forward` required for inter-VPC access.

## Supported Platforms

| Platform | Mechanism | Max IPs/node |
|---|---|---|
| GKE | VPC alias IP ranges (`gcloud`) | ~254 |
| Volcengine | Dedicated subnet + secondary ENI IPs (`ve` CLI) | 140 (7 ENIs × 20) |

## What It Does

1. **Detects** the cloud platform via instance metadata (auto, or `--platform` flag).
2. **Provisions** cloud networking:
   - **GKE**: adds a secondary IP range to the subnet, assigns alias IPs to the instance NIC, fixes GCE guest-agent route hijack.
   - **Volcengine**: creates a dedicated /24 subnet, creates and attaches 7 secondary ENIs, assigns 20 secondary private IPs per ENI.
3. **Configures** the node:
   - Creates `cni0` bridge with gateway IP.
   - Generates `/etc/dnsmasq-cni.d/cni0.conf` with contiguous DHCP ranges and restarts `dnsmasq-cni`.
   - Adds `/32` host routes for each VM IP pointing to `cni0`.
   - Installs iptables FORWARD rules between secondary NICs and `cni0`, plus NAT MASQUERADE for outbound.
   - Applies sysctl (`ip_forward=1`, `rp_filter=0`).
4. **Generates** `/etc/cni/net.d/30-dnsmasq-dhcp.conflist`.
5. **Saves** pool state to `/var/lib/cocoon/net/pool.json`.

## Installation

```bash
curl -sL https://github.com/cocoonstack/cocoon-net/releases/latest/download/cocoon-net_Linux_x86_64.tar.gz | tar xz
sudo install -m 0755 cocoon-net /usr/local/bin/
```

## CLI Usage

### init — full node setup

```bash
sudo cocoon-net init \
  --node-name cocoon-pool \
  --subnet 172.20.100.0/24 \
  --pool-size 140
```

With all flags:

```bash
sudo cocoon-net init \
  --node-name cocoon-pool \
  --subnet 172.20.100.0/24 \
  --pool-size 140 \
  --gateway 172.20.100.1 \
  --platform volcengine \
  --primary-nic eth0 \
  --dns "8.8.8.8,1.1.1.1" \
  --state-dir /var/lib/cocoon/net
```

Dry run (show what would be done):

```bash
sudo cocoon-net init --node-name cocoon-pool --subnet 172.20.100.0/24 --dry-run
```

### status — show pool state

```bash
sudo cocoon-net status
```

Output:

```
Platform:   volcengine
Node:       cocoon-pool
Subnet:     172.20.100.0/24
Gateway:    172.20.100.1
IPs:        140
Updated:    2026-04-04T06:00:00Z
ENIs:       7
SubnetID:   subnet-xxx
```

### teardown — remove cloud networking

```bash
sudo cocoon-net teardown
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `--platform` | auto-detect | Force platform (`gke` or `volcengine`) |
| `--node-name` | (required for init) | Virtual node name (e.g. `cocoon-pool`) |
| `--subnet` | (required for init) | VM subnet CIDR (e.g. `172.20.100.0/24`) |
| `--pool-size` | `140` | Number of IPs to provision |
| `--gateway` | first IP in subnet | Gateway IP on `cni0` |
| `--primary-nic` | auto-detect | Host primary NIC (`ens4` on GKE, `eth0` on Volcengine) |
| `--dns` | `8.8.8.8,1.1.1.1` | Comma-separated DNS servers for DHCP clients |
| `--state-dir` | `/var/lib/cocoon/net` | State directory for `pool.json` |
| `--dry-run` | `false` | Show what would be done, without making changes |

## Credentials

### GKE

Uses application default credentials or the GCE instance service account. Requires `roles/compute.networkAdmin` or equivalent.

### Volcengine

Reads from `~/.volcengine/config.json`:

```json
{
  "access_key_id": "AKxxxx",
  "secret_access_key": "xxxx",
  "region": "cn-hongkong"
}
```

Or environment variables:

```bash
export VOLCENGINE_ACCESS_KEY_ID=AKxxxx
export VOLCENGINE_SECRET_ACCESS_KEY=xxxx
export VOLCENGINE_REGION=cn-hongkong
```

## Pool State File

`/var/lib/cocoon/net/pool.json`:

```json
{
  "platform": "volcengine",
  "nodeName": "cocoon-pool",
  "subnet": "172.20.100.0/24",
  "gateway": "172.20.100.1",
  "ips": ["172.20.100.2", "172.20.100.3", ...],
  "eniIDs": ["eni-xxx", ...],
  "subnetID": "subnet-xxx",
  "updatedAt": "2026-04-04T06:00:00Z"
}
```

## CNI Integration

`cocoon-net init` generates `/etc/cni/net.d/30-dnsmasq-dhcp.conflist`:

```json
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
```

The CNI IPAM is intentionally empty — Windows guests obtain their IP directly from dnsmasq running on `cni0`. Using `"type": "dhcp"` would cause a dual-DHCP conflict.

In a CocoonSet, use `network: dnsmasq-dhcp` to route VM pods through this CNI:

```yaml
spec:
  agent:
    network: dnsmasq-dhcp
    os: windows
```

## Architecture

```
cocoon-net/
├── main.go                    # CLI entry point (cobra)
├── version/version.go         # version info (ldflags)
├── cmd/
│   ├── root.go                # root command
│   ├── init_cmd.go            # cocoon-net init
│   ├── status_cmd.go          # cocoon-net status
│   └── teardown_cmd.go        # cocoon-net teardown
├── platform/
│   ├── platform.go            # CloudPlatform interface + types
│   ├── detect.go              # auto-detect GKE vs Volcengine
│   ├── gke.go                 # GKE implementation (alias IPs via gcloud)
│   └── volcengine.go          # Volcengine implementation (subnet + ENI via ve)
├── node/
│   ├── node.go                # Setup() orchestrator + CNI conflist
│   ├── bridge.go              # cni0 bridge setup
│   ├── dnsmasq.go             # dnsmasq config generation
│   ├── routes.go              # host routes for VM IPs
│   ├── iptables.go            # iptables FORWARD + NAT rules
│   └── sysctl.go              # sysctl settings
├── pool/
│   ├── pool.go                # IP pool state management
│   └── pool.json              # pool state schema example
└── docs/
    ├── gke.md                 # GKE networking guide
    └── volcengine.md          # Volcengine networking guide
```

## Development

```bash
make build        # build binary
make test         # run tests
make lint         # run golangci-lint
make fmt          # format with gofumpt + goimports
make fmt-check    # check formatting (CI)
make deps         # go mod tidy
make clean        # remove artifacts
```

## Guides

- [GKE VPC-native networking](docs/gke.md)
- [Volcengine VPC-native networking](docs/volcengine.md)
