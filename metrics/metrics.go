// Package metrics defines the prometheus collectors for cocoon-net.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "cocoon"
	subsystem = "net"
)

// DHCPLeaseTotal counts lease grant attempts by result, once per REQUEST that names an IP.
var DHCPLeaseTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "dhcp_lease_total",
		Help:      "Number of DHCP lease grant attempts by result.",
	},
	[]string{"result"},
)

// Register installs the static collectors; the pool collector is registered separately.
func Register(reg prometheus.Registerer) {
	reg.MustRegister(DHCPLeaseTotal)
}
