# Configuration

cocoon-net is configured through command-line flags plus the environment
variables below; runtime state (what was provisioned, and for whom) lives in
`pool.json`.

## Flags

| Flag | Default | Description |
|---|---|---|
| `--platform` | auto-detect | Cloud platform (`gke` or `volcengine`); auto-detected from instance metadata if omitted |
| `--node-name` | (required) | Virtual node name |
| `--subnet` | (required) | VM subnet CIDR (e.g. `172.20.100.0/24`) |
| `--pool-size` | `140` (init) / `253` (adopt) | Number of IPs in the pool; read by GKE `init` and `adopt`, ignored on Volcengine (the pool is the ENI secondary IPs) |
| `--gateway` | first IP in subnet | Gateway IP on `cni0` |
| `--primary-nic` | `eth0` (Volcengine) / `ens4` (other platforms) | Host primary NIC |
| `--dns` | `8.8.8.8,1.1.1.1` | DNS servers for DHCP clients |
| `--state-dir` | `/var/lib/cocoon/net` | State directory for `pool.json` |
| `--lease-file` | `<state-dir>/leases.json` | (daemon) DHCP lease persistence file |
| `--control-socket` | `/run/cocoon-net/control.sock` | (daemon) Root-only Unix socket used by local VM lifecycle managers to reclaim leases; empty to disable |
| `--drop-cidr` | none | (repeatable, `init`/`adopt`) Destination CIDR to DROP at `FORWARD` for VM traffic -- see [DHCP: traffic isolation](dhcp.md#traffic-isolation) |
| `--drop-internal-access` | `false` | (`init`/`adopt`) DROP `FORWARD` traffic within the node's own `--subnet` |
| `--dry-run` | `false` | (`init`/`adopt`/`teardown`) Preview changes without applying |
| `--skip-iptables` | `false` | (daemon) Skip iptables setup |
| `--manage-iptables` | `false` | (adopt) Let cocoon-net write iptables rules |
| `--metrics-addr` | `:9092` | (daemon) Prometheus listen address for `/metrics`; empty to disable |

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `COCOON_NET_LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `COCOON_NET_METRICS_ADDR` | `:9092` | Prometheus metrics listen address (overridden by `--metrics-addr`) |
| `COCOON_NET_CONTROL_SOCKET` | `/run/cocoon-net/control.sock` | Unix socket for the local lease lifecycle API (overridden by `--control-socket`) |

## Metrics

The daemon serves Prometheus metrics on `--metrics-addr` (`:9092` by
default) at `/metrics`:

| Metric | Type | Description |
|---|---|---|
| `cocoon_net_dhcp_lease_total{result}` | counter | DHCP lease-grant attempts by outcome (`ok`/`failed`) |
| `cocoon_net_dhcp_pool_available` | gauge | Unallocated IPs in the DHCP pool |
| `cocoon_net_dhcp_lease_active` | gauge | Active (unexpired) DHCP leases |

## Pool state (`pool.json`)

`init` and `adopt` persist the provisioned pool to `<state-dir>/pool.json`
(atomic tmp+rename write); `daemon`, `status`, and `teardown` all read it
back. Example (Volcengine):

```json
{
  "platform": "volcengine",
  "nodeName": "cocoon-pool",
  "subnet": "172.20.100.0/24",
  "gateway": "172.20.100.1",
  "ips": ["172.20.100.2", "172.20.100.3"],
  "eniIDs": ["eni-xxx"],
  "updatedAt": "2026-04-04T06:00:00Z"
}
```

Example file: [docs/pool-example.json](pool-example.json).

| Field | Description |
|---|---|
| `platform` | `gke` or `volcengine` |
| `nodeName` | Virtual node name (`--node-name`) |
| `subnet` | VM subnet CIDR |
| `gateway` | Gateway IP on `cni0` |
| `primaryNIC` | Host primary NIC |
| `secondaryNICs` | Volcengine only: `eth1`..`eth7` |
| `ips` | Allocatable DHCP pool: on GKE `init`/`adopt` it is up to `--pool-size` host addresses from the subnet, skipping the gateway and broadcast address; on Volcengine it is the secondary IPs the ENIs report |
| `eniIDs` | Volcengine only: the pool ENIs `init` or `adopt` recorded; `teardown` deletes exactly these |
| `aliasRangeName` | GKE only: the GCE secondary range the alias was bound from; empty for other platforms or adopted nodes |
| `dnsServers` | DNS servers handed out by DHCP; empty on state written before this field existed (daemon falls back to built-in defaults) |
| `dropInternalAccess`, `dropCIDRs` | Mirrors `--drop-internal-access` / `--drop-cidr`, reapplied by the daemon on every start unless `--skip-iptables` is set |
| `updatedAt` | Last write time (UTC) |

## Credentials

**GKE**: application default credentials, or the GCE instance service
account (`roles/compute.networkAdmin`).

**Volcengine**: `~/.volcengine/config.json`, or the
`VOLCENGINE_ACCESS_KEY_ID` / `VOLCENGINE_SECRET_ACCESS_KEY` /
`VOLCENGINE_REGION` environment variables.
