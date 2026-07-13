# cocoon-net

VPC-native networking for [Cocoon](https://github.com/cocoonstack/cocoon) VM
nodes. Provisions cloud networking resources and runs an embedded DHCP
server so VMs obtain VPC-routable IPs directly -- no overlay network, no
iptables DNAT, no external DHCP server dependency.

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

**Two-phase operation**: `cocoon-net init` (or `adopt`) does one-time cloud
provisioning and state persistence; `cocoon-net daemon` is the long-running
service that sets up node networking, serves DHCP, and manages routes.
Full details in [Architecture](docs/architecture.md).

## Supported platforms

| Platform | Mechanism | Max IPs/node |
|---|---|---|
| GKE | VPC alias IP ranges (`gcloud`) | ~254 |
| Volcengine | Dedicated subnet + secondary ENI IPs (`ve` CLI) | 140 (7 ENIs x 20) |

## Quick start

```bash
sudo install -m 0755 cocoon-net /usr/local/bin/
sudo cocoon-net init --platform gke --node-name cocoon-pool --subnet 172.20.100.0/24 --pool-size 140
sudo cocoon-net daemon
```

Full steps in [Installation](docs/installation.md).

## Documentation

- [Architecture](docs/architecture.md) -- the two-phase model, package
  layout, CNI integration, credentials
- [Embedded DHCP server](docs/dhcp.md) -- lease lifecycle, the lease file,
  dynamic host routes, VM traffic isolation
- [Configuration](docs/configuration.md) -- every flag and environment
  variable, plus the `pool.json` state schema
- [Installation](docs/installation.md) -- prebuilt binary, building from
  source, the systemd unit, command usage
- [GKE VPC-native networking](docs/gke.md)
- [Volcengine VPC-native networking](docs/volcengine.md)

## Development

```bash
make build      # build binary
make test       # run tests with coverage
make lint       # golangci-lint (linux + darwin)
make fmt        # gofumpt + goimports
make help       # show all targets
```

## Related projects

| Project | Role |
|---|---|
| [cocoon](https://github.com/cocoonstack/cocoon) | MicroVM engine (Cloud Hypervisor + Firecracker) |
| [cocoon-common](https://github.com/cocoonstack/cocoon-common) | Shared metadata, Kubernetes, and logging helpers |
| [cocoon-operator](https://github.com/cocoonstack/cocoon-operator) | CocoonSet and Hibernation CRDs |
| [cocoon-webhook](https://github.com/cocoonstack/cocoon-webhook) | Admission webhook for sticky scheduling |
| [vk-cocoon](https://github.com/cocoonstack/vk-cocoon) | Virtual kubelet provider |

## License

[MIT](LICENSE)
