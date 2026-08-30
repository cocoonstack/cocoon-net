package dhcp

import (
	"context"
	"time"

	"github.com/projecteru2/core/log"
)

func (s *Server) restoreLeases(ctx context.Context) {
	logger := log.WithFunc("dhcp.restoreLeases")
	restored := 0
	for _, l := range s.leases.activeLeases() {
		if !s.pool.markUsed(l.IP) {
			s.leases.take(l.MAC)
			if err := delRouteFn(l.IP, s.linkIndex); err != nil {
				logger.Warnf(ctx, "del route %s: %v", l.IP, err)
			}
			logger.Warnf(ctx, "dropped lease %s <- %s: outside the pool", l.IP, l.MAC)
			continue
		}
		if err := addRouteFn(l.IP, s.linkIndex); err != nil {
			logger.Errorf(ctx, err, "restore route %s", l.IP)
		}
		restored++
	}
	if restored > 0 {
		logger.Infof(ctx, "restored %d active leases", restored)
	}
}

func (s *Server) cleanupLoop(ctx context.Context) {
	logger := log.WithFunc("dhcp.cleanupLoop")
	ticker := time.NewTicker(leaseCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.lifecycleMu.Lock()
			for _, ip := range s.offers.expireOld() {
				s.pool.release(ip)
				logger.Infof(ctx, "reclaimed abandoned offer %s", ip)
			}

			expired := s.leases.expireOld()
			if len(expired) > 0 {
				if err := s.leases.save(); err != nil {
					s.leases.restoreAll(expired)
					logger.Error(ctx, err, "persist leases before cleanup")
					s.lifecycleMu.Unlock()
					continue
				}
			}
			for _, l := range expired {
				s.freeIP(ctx, l.IP)
				logger.Infof(ctx, "expired lease %s <- %s", l.IP, l.MAC)
			}
			s.lifecycleMu.Unlock()
		}
	}
}
