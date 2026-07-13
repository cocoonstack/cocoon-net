# Configuration

cocoon-net is configured through command-line flags plus one environment
variable; runtime state (what was provisioned, and for whom) lives in
`pool.json`.

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
| `--drop-cidr` | none | (repeatable, `init`/`adopt`) Destination CIDR to DROP at `FORWARD` for VM traffic -- see [DHCP: traffic isolation](dhcp.md#traffic-isolation) |
| `--drop-internal-access` | `false` | (`init`/`adopt`) DROP `FORWARD` traffic within the node's own `--subnet` |
| `--dry-run` | `false` | Preview changes without applying |
| `--skip-iptables` | `false` | (daemon) Skip iptables setup |
| `--manage-iptables` | `false` | (adopt) Let cocoon-net write iptables rules |

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `COCOON_NET_LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |

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
  "subnetID": "subnet-xxx",
  "updatedAt": "2026-04-04T06:00:00Z"
}
```

Full example file: [docs/pool-example.json](pool-example.json).

| Field | Description |
|---|---|
| `platform` | `gke` or `volcengine` |
| `nodeName` | Virtual node name (`--node-name`) |
| `subnet` | VM subnet CIDR |
| `gateway` | Gateway IP on `cni0` |
| `primaryNIC` | Host primary NIC |
| `secondaryNICs` | Volcengine only: `eth1`..`eth7` |
| `ips` | Allocatable DHCP pool (excludes the gateway) |
| `eniIDs` | Volcengine only: attached ENI IDs |
| `subnetID` | Volcengine only: the VM subnet's ID |
| `aliasRangeName` | GKE only: the GCE secondary range the alias was bound from; empty for other platforms or adopted nodes |
| `dnsServers` | DNS servers handed out by DHCP; empty on state written before this field existed (daemon falls back to built-in defaults) |
| `dropInternalAccess`, `dropCIDRs` | Mirrors `--drop-internal-access` / `--drop-cidr`, reapplied by the daemon on every start |
| `updatedAt` | Last write time (UTC) |

## Credentials

**GKE**: application default credentials, or the GCE instance service
account (`roles/compute.networkAdmin`).

**Volcengine**: `~/.volcengine/config.json`, or the
`VOLCENGINE_ACCESS_KEY_ID` / `VOLCENGINE_SECRET_ACCESS_KEY` /
`VOLCENGINE_REGION` environment variables.
