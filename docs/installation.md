# Installation

## Prebuilt binary

```bash
VER=$(curl -sI https://github.com/cocoonstack/cocoon-net/releases/latest | awk -F'/v' 'tolower($0) ~ /^location:/ {print $NF}' | tr -d '\r')
ARCH=$([ "$(uname -m)" = "aarch64" ] && echo arm64 || echo x86_64)
curl -sL "https://github.com/cocoonstack/cocoon-net/releases/download/v${VER}/cocoon-net_${VER}_Linux_${ARCH}.tar.gz" | tar xz
sudo install -m 0755 cocoon-net /usr/local/bin/
```

## Build from source

```bash
git clone https://github.com/cocoonstack/cocoon-net.git
cd cocoon-net
make build
```

## Usage

`cocoon-net` runs in two phases: a one-time provisioning command, then a
long-lived daemon. See [Architecture](architecture.md) for how they fit
together.

```bash
# 1. provision cloud networking (or `adopt` an already-provisioned node)
sudo cocoon-net init \
  --platform gke \
  --node-name cocoon-pool \
  --subnet 172.20.100.0/24 \
  --pool-size 140

# 2. run the DHCP server + node networking as a daemon
sudo cocoon-net daemon

# inspect pool state
cocoon-net status

# remove cloud networking resources (also deletes pool.json + leases.json)
sudo cocoon-net teardown
```

Platform-specific `init`/`adopt` walkthroughs, prerequisites, and
troubleshooting live in [GKE](gke.md) and [Volcengine](volcengine.md). Every
flag and environment variable is documented in
[Configuration](configuration.md).

### `adopt` -- bring an existing node under management

For nodes whose cloud networking was already provisioned by hand:

```bash
sudo cocoon-net adopt \
  --platform gke \
  --node-name cocoon-pool \
  --subnet 172.20.0.0/24
```

## Systemd unit

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

Building from source and running tests: see [Development](../README.md#development)
in the README.
