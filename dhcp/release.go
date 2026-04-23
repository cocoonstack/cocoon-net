package dhcp

import (
	"context"
	"net"

	"github.com/projecteru2/core/log"
)

func (s *Server) handleRelease(ctx context.Context, mac net.HardwareAddr) {
	logger := log.WithFunc("dhcp.handleRelease")

	ip := s.leases.ipForMAC(mac)
	if ip == nil {
		return
	}

	s.leases.remove(mac)
	s.pool.release(ip)

	if err := delRoute(ip, s.linkIndex); err != nil {
		logger.Errorf(ctx, err, "del route %s", ip)
	}

	if err := s.leases.save(); err != nil {
		logger.Errorf(ctx, err, "persist leases after RELEASE")
	}
	logger.Infof(ctx, "RELEASE %s <- %s", ip, mac)
}
