package dhcp

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/projecteru2/core/log"
)

// ReleaseLease reclaims mac's lease, route and pool slot; idempotent so lifecycle callers can retry.
func (s *Server) ReleaseLease(ctx context.Context, mac net.HardwareAddr) (bool, error) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.releaseLeaseLocked(ctx, mac)
}

func (s *Server) releaseLeaseLocked(ctx context.Context, mac net.HardwareAddr) (bool, error) {
	logger := log.WithFunc("dhcp.releaseLeaseLocked")
	l := s.leases.take(mac)
	if l == nil {
		return false, nil
	}

	// persist before freeing the IP; on failure restore the entry so a retry cannot double-allocate after restart
	if err := s.leases.save(); err != nil {
		s.leases.restoreAll([]lease{*l})
		return true, fmt.Errorf("persist leases: %w", err)
	}
	s.freeIP(ctx, l.IP)
	logger.Infof(ctx, "RELEASE %s <- %s", l.IP, mac)
	return true, nil
}

// freeIP deletes the /32 before releasing: a crash in between must not leave a route no restart path removes.
func (s *Server) freeIP(ctx context.Context, ip net.IP) {
	logger := log.WithFunc("dhcp.freeIP")
	if err := delRouteFn(ip, s.linkIndex); err != nil {
		logger.Errorf(ctx, err, "del route %s", ip)
	}
	s.pool.release(ip)
}

func (s *Server) handleRelease(ctx context.Context, peer net.Addr, msg *dhcpv4.DHCPv4, mac net.HardwareAddr, arrived time.Time) {
	logger := log.WithFunc("dhcp.handleRelease")
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	l := s.leases.active(mac)
	if l == nil {
		return
	}
	// chaddr is client-chosen and macspoofchk pins only the L2 source, so the release must come from the address it frees
	if src, ok := peer.(*net.UDPAddr); !ok || !src.IP.Equal(l.IP) || !msg.ClientIPAddr.Equal(l.IP) {
		logger.Warnf(ctx, "ignored RELEASE of %s <- %s from %s", l.IP, mac, peer)
		return
	}
	if l.Granted.After(arrived) {
		logger.Infof(ctx, "ignored RELEASE of %s <- %s: a later REQUEST holds the lease", l.IP, mac)
		return
	}
	if _, err := s.releaseLeaseLocked(ctx, mac); err != nil {
		logger.Error(ctx, err, "release lease")
	}
}
