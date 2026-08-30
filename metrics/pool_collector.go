package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// PoolState is a per-scrape snapshot of pool occupancy, so the gauges vanish with the daemon.
type PoolState struct {
	Available int
	Active    int
}

// PoolStateFunc samples the live pool on every scrape.
type PoolStateFunc func() PoolState

var _ prometheus.Collector = (*PoolCollector)(nil)

// PoolCollector emits DHCP pool gauges by reading live server state per scrape.
type PoolCollector struct {
	stateFn PoolStateFunc

	availableDesc *prometheus.Desc
	activeDesc    *prometheus.Desc
}

// NewPoolCollector creates a collector; stateFn is called on every scrape.
func NewPoolCollector(stateFn PoolStateFunc) *PoolCollector {
	return &PoolCollector{
		stateFn:       stateFn,
		availableDesc: prometheus.NewDesc(prometheus.BuildFQName(namespace, subsystem, "dhcp_pool_available"), "Number of unallocated IPs in the DHCP pool.", nil, nil),
		activeDesc:    prometheus.NewDesc(prometheus.BuildFQName(namespace, subsystem, "dhcp_lease_active"), "Number of active (unexpired) DHCP leases.", nil, nil),
	}
}

func (c *PoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.availableDesc
	ch <- c.activeDesc
}

func (c *PoolCollector) Collect(ch chan<- prometheus.Metric) {
	st := c.stateFn()
	ch <- prometheus.MustNewConstMetric(c.availableDesc, prometheus.GaugeValue, float64(st.Available))
	ch <- prometheus.MustNewConstMetric(c.activeDesc, prometheus.GaugeValue, float64(st.Active))
}
