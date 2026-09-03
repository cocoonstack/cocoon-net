package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// PoolGauges samples the live DHCP pool on every scrape, so the gauges vanish with the daemon.
func PoolGauges(available, active func() int) []prometheus.Collector {
	return []prometheus.Collector{
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: subsystem,
			Name: "dhcp_pool_available", Help: "Number of unallocated IPs in the DHCP pool.",
		}, func() float64 { return float64(available()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: subsystem,
			Name: "dhcp_lease_active", Help: "Number of active (unexpired) DHCP leases.",
		}, func() float64 { return float64(active()) }),
	}
}
