# cocoon-net

VPC-native networking setup for [Cocoon](https://github.com/cocoonstack/cocoon) VM nodes on GKE and Volcengine. It provisions cloud networking resources and configures the Linux host so that Windows and Linux VMs obtain VPC-routable IPs directly via DHCP — no overlay network, no iptables DNAT, no `kubectl port-forward` required.

## Overview

- **Platform auto-detection** -- identifies GKE or Volcengine via instance metadata
- **Cloud resource provisioning** -- GKE alias IP ranges or Volcengine ENI secondary IPs
- **Host networking** -- cni0 bridge, dnsmasq DHCP, /32 host routes, iptables FORWARD + NAT
- **CNI integration** -- generates conflist for Kubernetes pod networking
- **State management** -- persists pool state to `/var/lib/cocoon/net/pool.json`
- **Adopt mode** -- bring existing hand-provisioned nodes under management without cloud API calls
- **Idempotent re-runs** -- skips config writes and service restarts when nothing changed
- **Dry-run mode** -- preview all changes before applying

### Supported platforms

| Platform | Mechanism | Max IPs/node |
|---|---|---|
| GKE | VPC alias IP ranges (`gcloud`) | ~254 |
| Volcengine | Dedicated subnet + secondary ENI IPs (`ve` CLI) | 140 (7 ENIs x 20) |

## Architecture

On each node, `cocoon-net init` runs through these steps:

1. **Detect** the cloud platform via instance metadata (auto, or `--platform` flag).
2. **Provision** cloud networking:
   - **GKE**: adds a secondary IP range to the subnet, assigns alias IPs to the instance NIC, fixes GCE guest-agent route hijack.
   - **Volcengine**: creates a dedicated /24 subnet, creates and attaches 7 secondary ENIs, assigns 20 secondary private IPs per ENI.
3. **Configure** the node:
   - Creates `cni0` bridge with gateway IP.
   - Generates `/etc/dnsmasq-cni.d/cni0.conf` with contiguous DHCP ranges and restarts `dnsmasq-cni`.
   - Adds `/32` host routes for each VM IP pointing to `cni0`.
   - Installs iptables FORWARD rules between secondary NICs and `cni0`, plus NAT MASQUERADE for outbound.
   - Applies sysctl (`ip_forward=1`, `rp_filter=0`).
4. **Generate** `/etc/cni/net.d/30-dnsmasq-dhcp.conflist`.
5. **Save** pool state to `/var/lib/cocoon/net/pool.json`.

## Installation

### Download

```bash
curl -sL https://github.com/cocoonstack/cocoon-net/releases/latest/download/cocoon-net_Linux_x86_64.tar.gz | tar xz
sudo install -m 0755 cocoon-net /usr/local/bin/
```

### Build from source

```bash
git clone https://github.com/cocoonstack/cocoon-net.git
cd cocoon-net
make build          # produces ./cocoon-net
```

## Configuration

### Flags

| Flag | Default | Description |
|---|---|---|
| `--platform` | auto-detect | Force platform (`gke` or `volcengine`) |
| `--node-name` | (required) | Virtual node name (e.g. `cocoon-pool`) |
| `--subnet` | (required) | VM subnet CIDR (e.g. `172.20.100.0/24`) |
| `--pool-size` | `140` (init) / `253` (adopt) | Number of IPs in the pool |
| `--gateway` | first IP in subnet | Gateway IP on `cni0` |
| `--primary-nic` | auto-detect | Host primary NIC (`ens4` on GKE, `eth0` on Volcengine) |
| `--dns` | `8.8.8.8,1.1.1.1` | Comma-separated DNS servers for DHCP clients |
| `--state-dir` | `/var/lib/cocoon/net` | State directory for `pool.json` |
| `--dry-run` | `false` | Show what would be done, without making changes |
| `--manage-iptables` | `false` | (adopt only) Let cocoon-net write FORWARD + NAT rules |

### Environment variables

| Variable | Default | Description |
|---|---|---|
| `COCOON_NET_LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |

### Credentials

**GKE**

Uses application default credentials or the GCE instance service account. Requires `roles/compute.networkAdmin` or equivalent.

**Volcengine**

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

## Usage

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

### adopt — bring an existing node under cocoon-net management

For nodes whose cloud networking (alias IP range or secondary ENIs) was already provisioned by hand, `adopt` runs only the host-side configuration (bridge, sysctl, routes, dnsmasq, CNI conflist) and writes the pool state file — without calling any cloud APIs.

```bash
sudo cocoon-net adopt \
  --node-name cocoon-pool \
  --subnet 172.20.0.0/24
```

By default, adopt preserves the host's existing iptables rules. Opt in to let cocoon-net manage them:

```bash
sudo cocoon-net adopt \
  --node-name cocoon-pool \
  --subnet 172.20.0.0/24 \
  --manage-iptables
```

Cloud-side teardown must be done manually on adopted nodes — cocoon-net will not undo what it did not provision.

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

### CNI integration

Both `init` and `adopt` generate `/etc/cni/net.d/30-dnsmasq-dhcp.conflist`:

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

## Development

```bash
make build          # build binary
make test           # run tests with coverage
make lint           # run golangci-lint (linux + darwin)
make fmt            # format code with gofumpt and goimports
make deps           # tidy modules
make clean          # remove artifacts
make help           # show all targets
```

### Guides

- [GKE VPC-native networking](docs/gke.md)
- [Volcengine VPC-native networking](docs/volcengine.md)

## Related Projects

| Project | Role |
|---|---|
| [cocoon-common](https://github.com/cocoonstack/cocoon-common) | Shared metadata, Kubernetes, and logging helpers |
| [cocoon-operator](https://github.com/cocoonstack/cocoon-operator) | CocoonSet and Hibernation CRDs |
| [cocoon-webhook](https://github.com/cocoonstack/cocoon-webhook) | Admission webhook for sticky scheduling |
| [epoch](https://github.com/cocoonstack/epoch) | Snapshot storage backend |
| [vk-cocoon](https://github.com/cocoonstack/vk-cocoon) | Virtual kubelet provider managing VM lifecycle |

## License

[MIT](LICENSE)
