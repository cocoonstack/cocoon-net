package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPoolCollector(t *testing.T) {
	t.Parallel()

	c := NewPoolCollector(func() PoolState {
		return PoolState{Available: 5, Active: 3}
	})
	want := `
# HELP cocoon_net_dhcp_lease_active Number of active (unexpired) DHCP leases.
# TYPE cocoon_net_dhcp_lease_active gauge
cocoon_net_dhcp_lease_active 3
# HELP cocoon_net_dhcp_pool_available Number of unallocated IPs in the DHCP pool.
# TYPE cocoon_net_dhcp_pool_available gauge
cocoon_net_dhcp_pool_available 5
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want)); err != nil {
		t.Errorf("collect mismatch: %v", err)
	}
}
