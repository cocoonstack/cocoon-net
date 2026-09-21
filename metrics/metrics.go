// Package metrics defines the prometheus collectors for cocoon-net.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "cocoon"
	subsystem = "net"
)

var (
	// DHCPLeaseTotal counts lease grant attempts by result, once per REQUEST that names an IP.
	DHCPLeaseTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "dhcp_lease_total",
			Help:      "Number of DHCP lease grant attempts by result.",
		},
		[]string{"result"},
	)

	// SecondaryNICs reports the pool's secondary NICs by state so a missing ENI is visible.
	SecondaryNICs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "secondary_nics",
			Help:      "Secondary NICs the pool expects and the host presents.",
		},
		[]string{"state"},
	)
)

// Register installs the static collectors; the pool collector is registered separately.
func Register(reg prometheus.Registerer) {
	reg.MustRegister(DHCPLeaseTotal, SecondaryNICs)
}
