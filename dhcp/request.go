package dhcp

import (
	"context"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/projecteru2/core/log"

	"github.com/cocoonstack/cocoon-net/metrics"
)

func (s *Server) handleRequest(ctx context.Context, conn net.PacketConn, peer net.Addr, msg *dhcpv4.DHCPv4, mac net.HardwareAddr) {
	logger := log.WithFunc("dhcp.handleRequest")

	reqIP := msg.RequestedIPAddress()
	if reqIP == nil || reqIP.IsUnspecified() {
		reqIP = msg.ClientIPAddr
	}
	if reqIP == nil || reqIP.IsUnspecified() {
		logger.Warnf(ctx, "REQUEST from %s: no IP requested", mac)
		return
	}

	// Record exactly one outcome per grant attempt: default failed, flipped to
	// ok only once the ACK is on the wire.
	result := "failed"
	defer func() { metrics.DHCPLeaseTotal.WithLabelValues(result).Inc() }()

	if s.leases.isLeasedToOther(mac, reqIP) {
		s.sendNAK(ctx, conn, peer, msg)
		logger.Infof(ctx, "NAK %s -> %s (leased to another client)", reqIP, mac)
		return
	}

	alreadyHeld := s.offers.isOfferedTo(mac, reqIP) || s.leases.isLeasedTo(mac, reqIP)
	if !alreadyHeld && !s.pool.tryClaim(reqIP) {
		s.sendNAK(ctx, conn, peer, msg)
		logger.Infof(ctx, "NAK %s -> %s (not available)", reqIP, mac)
		return
	}

	resp, err := s.buildReply(msg, dhcpv4.MessageTypeAck, reqIP)
	if err != nil {
		if !alreadyHeld {
			s.pool.release(reqIP)
		}
		logger.Errorf(ctx, err, "build ACK for %s", mac)
		return
	}

	// Install the /32 route before committing lease state — without it
	// the client would lease an unreachable IP. RouteReplace is
	// idempotent so client re-REQUESTs are safe.
	if err := addRouteFn(reqIP, s.linkIndex); err != nil {
		if !alreadyHeld {
			s.pool.release(reqIP)
		}
		logger.Errorf(ctx, err, "add route %s; NAKing", reqIP)
		s.sendNAK(ctx, conn, peer, msg)
		return
	}

	if oldIP := s.offers.remove(mac); oldIP != nil && !oldIP.Equal(reqIP) {
		s.pool.release(oldIP)
	}
	for _, e := range s.leases.add(mac, reqIP, s.conf.LeaseTime) {
		if e.MAC == mac.String() {
			if err := delRouteFn(e.IP, s.linkIndex); err != nil {
				logger.Errorf(ctx, err, "del orphan route %s after %s rebind to %s", e.IP, mac, reqIP)
			}
			s.pool.release(e.IP)
			logger.Warnf(ctx, "rebound %s from %s to %s; released old IP", mac, e.IP, reqIP)
			continue
		}
		logger.Warnf(ctx, "evicted stale lease for %s held by %s while ACKing %s", reqIP, e.MAC, mac)
	}

	if _, err := conn.WriteTo(resp.ToBytes(), peer); err != nil {
		// Route + lease are committed; client will re-REQUEST and hit isLeasedTo.
		logger.Errorf(ctx, err, "send ACK to %s (committed; awaiting client retry)", mac)
		return
	}

	result = "ok"
	if err := s.leases.save(); err != nil {
		logger.Error(ctx, err, "persist leases after ACK")
	}
	logger.Infof(ctx, "ACK %s -> %s", reqIP, mac)
}
