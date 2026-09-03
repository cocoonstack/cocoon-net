package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPoolGauges(t *testing.T) {
	t.Parallel()

	gauges := PoolGauges(func() int { return 5 }, func() int { return 3 })
	want := []string{`
# HELP cocoon_net_dhcp_pool_available Number of unallocated IPs in the DHCP pool.
# TYPE cocoon_net_dhcp_pool_available gauge
cocoon_net_dhcp_pool_available 5
`, `
# HELP cocoon_net_dhcp_lease_active Number of active (unexpired) DHCP leases.
# TYPE cocoon_net_dhcp_lease_active gauge
cocoon_net_dhcp_lease_active 3
`}
	for i, g := range gauges {
		if err := testutil.CollectAndCompare(g, strings.NewReader(want[i])); err != nil {
			t.Errorf("gauge %d: %v", i, err)
		}
	}
}
