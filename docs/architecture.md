# Architecture

cocoon-net gives each cocoon VM a VPC-routable IP directly -- no overlay
network, no iptables DNAT, no external DHCP server. It runs as a two-phase
CLI: one-time cloud provisioning, then a long-lived daemon that serves
DHCP and keeps host routes in sync with leases.

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

1. `cocoon-net init` (or `adopt`) -- provisions cloud networking resources
   (GKE alias IPs or Volcengine secondary ENI IPs), configures the node, and
   persists the result to `pool.json`.
2. `cocoon-net daemon` -- long-running service: re-applies node setup, then
   starts the [embedded DHCP server](dhcp.md) that hands out pool IPs and
   manages dynamic host routes.

## Package model

| Package | Responsibility |
|---|---|
| `cmd/` | Cobra commands: `init`, `adopt`, `daemon`, `status`, `teardown` |
| `platform/` | `CloudPlatform` interface with per-cloud implementations (`platform/gke`, `platform/volcengine`); auto-detection via instance metadata |
| `pool/` | `pool.State` -- the allocation pool persisted to `pool.json` (atomic tmp+rename write) |
| `node/` | `cni0` bridge, sysctl, iptables FORWARD + NAT, and the CNI conflist writer |
| `dhcp/` | The embedded DHCPv4 server: lease store, IP pool, dynamic route add/remove -- see [DHCP server](dhcp.md) |

`platform.CloudPlatform` is the seam between cloud-specific provisioning and
everything else: `ProvisionNetwork` returns a `NetworkResult` (assigned IPs,
primary/secondary NICs, platform-specific IDs) that `pool.State` persists
verbatim, so `node/` and `dhcp/` never need to know which cloud they're
running on.

## Supported platforms

| Platform | Mechanism | Max IPs/node |
|---|---|---|
| GKE | VPC alias IP ranges (`gcloud`) | ~254 |
| Volcengine | Dedicated subnet + secondary ENI IPs (`ve` CLI) | 140 (7 ENIs x 20) |

Platform-specific setup, prerequisites, and troubleshooting:

- [GKE VPC-native networking](gke.md)
- [Volcengine VPC-native networking](volcengine.md)

> **GKE multi-node**: the secondary range `cocoon-pods` on the GCE subnet is
> shared across nodes. See [gke.md Prerequisites](gke.md#prerequisites)
> before running `init` on a second node.

## CNI integration

Both `init` and `adopt` write `/etc/cni/net.d/30-cocoon-dhcp.conflist`:

```json
{
  "cniVersion": "1.0.0",
  "name": "cocoon-dhcp",
  "plugins": [{
    "type": "bridge",
    "bridge": "cni0",
    "isGateway": false,
    "ipMasq": false,
    "portIsolation": true,
    "macspoofchk": true,
    "ipam": {}
  }]
}
```

`portIsolation` blocks same-node VM-to-VM traffic at L2 and `macspoofchk`
pins each veth's source MAC (see [Traffic isolation](dhcp.md)). IPAM is
intentionally empty -- VMs obtain their IP from the [embedded DHCP
server](dhcp.md), not from CNI. In a CocoonSet:

```yaml
spec:
  agent:
    network: cocoon-dhcp
    os: windows
```

Cloud credentials each platform needs are covered in
[Configuration](configuration.md#credentials).
