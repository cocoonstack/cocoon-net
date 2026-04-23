package dhcp

import (
	"context"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/projecteru2/core/log"
)

func (s *Server) handleDiscover(ctx context.Context, conn net.PacketConn, peer net.Addr, msg *dhcpv4.DHCPv4, mac net.HardwareAddr) {
	logger := log.WithFunc("dhcp.handleDiscover")

	// Re-offer existing lease first.
	ip := s.leases.ipForMAC(mac)
	if ip == nil {
		// Check if we already have a pending offer for this MAC.
		var staleIP net.IP
		ip, staleIP = s.offers.ipForMAC(mac)
		if staleIP != nil {
			s.pool.release(staleIP)
		}
	}
	if ip == nil {
		// Allocate a new IP from the free pool.
		ip = s.pool.allocate()
		if ip == nil {
			logger.Warnf(ctx, "DISCOVER from %s: pool exhausted", mac)
			return
		}
		// Track as pending offer (not yet committed as lease).
		// If this MAC had a stale offer for a different IP, release it.
		if oldIP := s.offers.add(mac, ip); oldIP != nil {
			s.pool.release(oldIP)
		}
	}

	resp, err := s.buildReply(msg, dhcpv4.MessageTypeOffer, ip)
	if err != nil {
		logger.Errorf(ctx, err, "build OFFER for %s", mac)
		return
	}

	if _, err := conn.WriteTo(resp.ToBytes(), peer); err != nil {
		logger.Errorf(ctx, err, "send OFFER to %s", mac)
		return
	}
	logger.Infof(ctx, "OFFER %s -> %s", ip, mac)
}
