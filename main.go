// Package main is the cocoon-net entry point. cocoon-net is the per-node
// network daemon: it auto-detects the cloud platform, provisions VM-side
// networking, and runs an in-cluster DHCP server for the cocoon IP pool.
package main

import (
	"github.com/cocoonstack/cocoon-net/cmd"
)

func main() {
	cmd.Execute()
}
