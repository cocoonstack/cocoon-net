package dhcp

import (
	"context"
	"fmt"
	"net"

	"github.com/projecteru2/core/log"
)

// ReleaseLease reclaims mac's lease, route, and pool slot. It is idempotent so
// VM lifecycle callers can safely retry after an uncertain response.
func (s *Server) ReleaseLease(ctx context.Context, mac net.HardwareAddr) (bool, error) {
	logger := log.WithFunc("dhcp.ReleaseLease")
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	l := s.leases.take(mac)
	if l == nil {
		return false, nil
	}

	// Persist the ownership change before making the IP reusable. If this
	// fails, restore the in-memory lease and leave the route/pool untouched so
	// a retry cannot create a duplicate allocation after daemon restart.
	if err := s.leases.save(); err != nil {
		s.leases.restore(l)
		return true, fmt.Errorf("persist leases: %w", err)
	}
	// delRoute before release: a concurrent DISCOVER may claim the IP as soon
	// as it returns to the pool, and a late route delete would black-hole it.
	if err := delRouteFn(l.IP, s.linkIndex); err != nil {
		logger.Errorf(ctx, err, "del route %s", l.IP)
	}
	s.pool.release(l.IP)
	logger.Infof(ctx, "RELEASE %s <- %s", l.IP, mac)
	return true, nil
}

func (s *Server) handleRelease(ctx context.Context, mac net.HardwareAddr) {
	logger := log.WithFunc("dhcp.handleRelease")
	if _, err := s.ReleaseLease(ctx, mac); err != nil {
		logger.Error(ctx, err, "release lease")
	}
}
