# cocoon-net

VPC-native networking for [Cocoon](https://github.com/cocoonstack/cocoon) VM nodes. Provisions cloud networking resources and runs an embedded DHCP server so VMs obtain VPC-routable IPs directly -- no overlay network, no iptables DNAT, no external DHCP server dependency.

## Overview

- **Embedded DHCP server** on `cni0` bridge, replacing the external DHCP server dependency
- **Dynamic /32 host routes** added on DHCP lease, removed on expiry
- **Platform auto-detection** via instance metadata (GKE or Volcengine)
- **Cloud resource provisioning** -- GKE alias IP ranges or Volcengine ENI secondary IPs
- **Host networking** -- cni0 bridge, sysctl, iptables FORWARD + NAT
- **CNI integration** -- generates conflist for Kubernetes pod networking
- **State management** -- pool state persisted to `/var/lib/cocoon/net/pool.json`
- **Adopt mode** -- bring existing hand-provisioned nodes under management
- **Daemon mode** -- runs as a long-lived systemd service

### Supported Platforms

| Platform | Mechanism | Max IPs/node |
|---|---|---|
| GKE | VPC alias IP ranges (`gcloud`) | ~254 |
| Volcengine | Dedicated subnet + secondary ENI IPs (`ve` CLI) | 140 (7 ENIs x 20) |

> **GKE multi-node**: the secondary range `cocoon-pods` on the GCE subnet is shared across nodes. For clusters with more than one node, pre-create it with a CIDR that covers every node's `--subnet` (e.g. `172.20.0.0/16` spanning `172.20.100.0/24`, `172.20.101.0/24`, ...). If the range does not exist when `init` runs, cocoon-net creates it at the caller's `--subnet`, which works for single-node but makes subsequent nodes with different `--subnet` values fail fast. See [docs/gke.md](docs/gke.md#prerequisites).

## Architecture

```
cocoon-net init          cocoon-net daemon
      |                        |
      v                        v
Cloud provisioning       Node setup (sysctl, bridge, iptables, CNI conflist)
(alias IPs / ENIs)             |
      |                        v
      v                  DHCP server on cni0
pool.json  <----------        |
                               v
                         On lease: add /32 route
                         On release: del /32 route
```

**Two-phase operation**:

1. `cocoon-net init` (or `adopt`) -- one-time cloud provisioning + state persistence
2. `cocoon-net daemon` -- long-running service: node setup + DHCP + dynamic routing

## Installation

```bash
curl -sL https://github.com/cocoonstack/cocoon-net/releases/latest/download/cocoon-net_Linux_x86_64.tar.gz | tar xz
sudo install -m 0755 cocoon-net /usr/local/bin/
```

Build from source:

```bash
git clone https://github.com/cocoonstack/cocoon-net.git
cd cocoon-net
make build
```

## Usage

### init -- provision cloud networking

```bash
sudo cocoon-net init \
  --platform gke \
  --node-name cocoon-pool \
  --subnet 172.20.100.0/24 \
  --pool-size 140
```

### daemon -- run DHCP server (systemd service)

```bash
sudo cocoon-net daemon
```

The daemon loads the pool from `pool.json`, configures host networking, and starts the embedded DHCP server. Host routes are managed dynamically: added when a VM gets a lease, removed when the lease expires.

Systemd unit:

```ini
[Unit]
Description=cocoon-net VPC networking daemon
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/cocoon-net daemon
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### adopt -- bring an existing node under management

For nodes whose cloud networking was already provisioned by hand:

```bash
sudo cocoon-net adopt \
  --platform gke \
  --node-name cocoon-pool \
  --subnet 172.20.0.0/24
```

### status -- show pool state

```bash
cocoon-net status
```

### teardown -- remove cloud networking resources

```bash
sudo cocoon-net teardown
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `--platform` | auto-detect | Cloud platform (`gke` or `volcengine`); auto-detected from instance metadata if omitted |
| `--node-name` | (required) | Virtual node name |
| `--subnet` | (required) | VM subnet CIDR (e.g. `172.20.100.0/24`) |
| `--pool-size` | `140` (init) / `253` (adopt) | Number of IPs in the pool |
| `--gateway` | first IP in subnet | Gateway IP on `cni0` |
| `--primary-nic` | auto-detect | Host primary NIC |
| `--dns` | `8.8.8.8,1.1.1.1` | DNS servers for DHCP clients |
| `--state-dir` | `/var/lib/cocoon/net` | State directory for `pool.json` |
| `--lease-file` | `/var/lib/cocoon/net/leases.json` | DHCP lease persistence file |
| `--dry-run` | `false` | Preview changes without applying |
| `--skip-iptables` | `false` | (daemon) Skip iptables setup |
| `--manage-iptables` | `false` | (adopt) Let cocoon-net write iptables rules |

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `COCOON_NET_LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |

## CNI Integration

Both `init` and `adopt` generate `/etc/cni/net.d/30-cocoon-dhcp.conflist`:

```json
{
  "cniVersion": "1.0.0",
  "name": "cocoon-dhcp",
  "plugins": [{
    "type": "bridge",
    "bridge": "cni0",
    "isGateway": false,
    "ipMasq": false,
    "ipam": {}
  }]
}
```

IPAM is intentionally empty -- VMs obtain IPs from the embedded DHCP server. In a CocoonSet:

```yaml
spec:
  agent:
    network: cocoon-dhcp
    os: windows
```

## Credentials

**GKE**: Uses application default credentials or GCE instance service account (`roles/compute.networkAdmin`).

**Volcengine**: Reads from `~/.volcengine/config.json` or environment variables (`VOLCENGINE_ACCESS_KEY_ID`, `VOLCENGINE_SECRET_ACCESS_KEY`, `VOLCENGINE_REGION`).

## Development

```bash
make build      # build binary
make test       # run tests with coverage
make lint       # golangci-lint (linux + darwin)
make fmt        # gofumpt + goimports
make help       # show all targets
```

### Guides

- [GKE VPC-native networking](docs/gke.md)
- [Volcengine VPC-native networking](docs/volcengine.md)

## Related Projects

| Project | Role |
|---|---|
| [cocoon](https://github.com/cocoonstack/cocoon) | MicroVM engine (Cloud Hypervisor + Firecracker) |
| [cocoon-common](https://github.com/cocoonstack/cocoon-common) | Shared metadata, Kubernetes, and logging helpers |
| [cocoon-operator](https://github.com/cocoonstack/cocoon-operator) | CocoonSet and Hibernation CRDs |
| [cocoon-webhook](https://github.com/cocoonstack/cocoon-webhook) | Admission webhook for sticky scheduling |
| [vk-cocoon](https://github.com/cocoonstack/vk-cocoon) | Virtual kubelet provider |

## License

[MIT](LICENSE)
