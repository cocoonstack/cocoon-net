# cocoon-net

VPC-native networking for [Cocoon](https://github.com/cocoonstack/cocoon) VM
nodes. cocoon-net provisions cloud networking resources and runs an
embedded DHCP server so VMs obtain VPC-routable IPs directly -- no overlay
network, no iptables DNAT, no external DHCP server dependency.

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

## Guides

- [Architecture](architecture.md) -- the two-phase model, the package
  layout (`platform` / `pool` / `node` / `dhcp`), CNI integration, and
  credentials
- [Embedded DHCP server](dhcp.md) -- lease lifecycle, the lease file,
  dynamic host routes, and VM traffic isolation (`--drop-cidr`)
- [Configuration](configuration.md) -- every flag and environment
  variable, plus the `pool.json` state schema
- [Installation](installation.md) -- prebuilt binary, building from
  source, the systemd unit, and command usage
- [GKE VPC-native networking](gke.md) -- alias IP ranges, multi-node
  setup, troubleshooting
- [Volcengine VPC-native networking](volcengine.md) -- secondary ENI IPs,
  setup, troubleshooting
