package dhcp

import (
	"context"
	"time"

	"github.com/projecteru2/core/log"
)

// restoreLeases re-adds /32 routes for all non-expired leases on startup.
func (s *Server) restoreLeases(ctx context.Context) {
	logger := log.WithFunc("dhcp.restoreLeases")
	active := s.leases.activeLeases()
	for _, l := range active {
		s.pool.markUsed(l.IP)
		if err := addRoute(l.IP, s.linkIndex); err != nil {
			logger.Errorf(ctx, err, "restore route %s", l.IP)
		}
	}
	if len(active) > 0 {
		logger.Infof(ctx, "restored %d active leases", len(active))
	}
}

// cleanupLoop periodically removes expired leases and abandoned offers.
func (s *Server) cleanupLoop(ctx context.Context) {
	logger := log.WithFunc("dhcp.cleanupLoop")
	ticker := time.NewTicker(leaseCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Reclaim abandoned offers.
			for _, ip := range s.offers.expireOld() {
				s.pool.release(ip)
				logger.Infof(ctx, "reclaimed abandoned offer %s", ip)
			}

			// Expire old leases.
			expired := s.leases.expireOld()
			for _, l := range expired {
				s.pool.release(l.IP)
				if err := delRoute(l.IP, s.linkIndex); err != nil {
					logger.Errorf(ctx, err, "del expired route %s", l.IP)
				}
				logger.Infof(ctx, "expired lease %s <- %s", l.IP, l.MAC)
			}
			if len(expired) > 0 {
				if err := s.leases.save(); err != nil {
					logger.Error(ctx, err, "persist leases after cleanup")
				}
			}
		}
	}
}
