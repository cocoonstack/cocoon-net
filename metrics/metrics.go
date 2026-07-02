// Package metrics defines the prometheus collectors for cocoon-net.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "cocoon"
	subsystem = "net"
)

// DHCPLeaseTotal counts DHCP lease-grant attempts by terminal outcome,
// recorded once per REQUEST that names an IP (result=ok|failed).
var DHCPLeaseTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "dhcp_lease_total",
		Help:      "Number of DHCP lease grant attempts by result.",
	},
	[]string{"result"},
)

// Register installs the static cocoon-net collectors on reg. The per-scrape
// pool collector is registered separately via NewPoolCollector.
func Register(reg prometheus.Registerer) {
	reg.MustRegister(DHCPLeaseTotal)
}
