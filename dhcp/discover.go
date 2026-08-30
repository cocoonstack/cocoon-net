package dhcp

import (
	"context"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/projecteru2/core/log"
)

func (s *Server) handleDiscover(ctx context.Context, conn net.PacketConn, peer net.Addr, msg *dhcpv4.DHCPv4, mac net.HardwareAddr) {
	logger := log.WithFunc("dhcp.handleDiscover")
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	ip := s.leases.ipForMAC(mac)
	if ip == nil {
		if ip = s.offerIP(mac); ip == nil {
			logger.Warnf(ctx, "DISCOVER from %s: pool exhausted", mac)
			return
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

// offerIP refreshes mac's live offer or allocates a new one; nil means the pool is exhausted.
func (s *Server) offerIP(mac net.HardwareAddr) net.IP {
	ip, staleIP := s.offers.ipForMAC(mac)
	if staleIP != nil {
		s.pool.release(staleIP)
	}
	if ip == nil {
		if ip = s.pool.allocate(); ip == nil {
			return nil
		}
	}
	if oldIP := s.offers.add(mac, ip); oldIP != nil {
		s.pool.release(oldIP)
	}
	return ip
}
