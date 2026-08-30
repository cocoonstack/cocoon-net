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
	s.freeIP(ctx, l.IP)
	logger.Infof(ctx, "RELEASE %s <- %s", l.IP, mac)
	return true, nil
}

// freeIP deletes the /32 first: a DISCOVER may claim the IP the moment it is back in the pool, and a late route delete would black-hole it.
func (s *Server) freeIP(ctx context.Context, ip net.IP) {
	if err := delRouteFn(ip, s.linkIndex); err != nil {
		log.WithFunc("dhcp.freeIP").Errorf(ctx, err, "del route %s", ip)
	}
	s.pool.release(ip)
}

func (s *Server) handleRelease(ctx context.Context, mac net.HardwareAddr) {
	logger := log.WithFunc("dhcp.handleRelease")
	if _, err := s.ReleaseLease(ctx, mac); err != nil {
		logger.Error(ctx, err, "release lease")
	}
}
