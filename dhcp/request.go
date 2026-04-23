package dhcp

import (
	"context"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/projecteru2/core/log"
)

func (s *Server) handleRequest(ctx context.Context, conn net.PacketConn, peer net.Addr, msg *dhcpv4.DHCPv4, mac net.HardwareAddr) {
	logger := log.WithFunc("dhcp.handleRequest")

	reqIP := msg.RequestedIPAddress()
	if reqIP == nil || reqIP.IsUnspecified() {
		reqIP = msg.ClientIPAddr
	}
	if reqIP == nil || reqIP.IsUnspecified() {
		logger.Infof(ctx, "REQUEST from %s: no IP requested", mac)
		return
	}

	// Validate: the IP must be either free, offered to this MAC, or already
	// leased to this MAC. Reject if it's leased to a different MAC.
	if s.leases.isLeasedToOther(mac, reqIP) {
		s.sendNAK(ctx, conn, peer, msg)
		logger.Infof(ctx, "NAK %s -> %s (leased to another client)", reqIP, mac)
		return
	}
	if !s.pool.isFree(reqIP) && !s.offers.isOfferedTo(mac, reqIP) && !s.leases.isLeasedTo(mac, reqIP) {
		s.sendNAK(ctx, conn, peer, msg)
		logger.Infof(ctx, "NAK %s -> %s (not available)", reqIP, mac)
		return
	}

	// Build the reply first. A buildReply failure here must not leave
	// pool/lease state or a /32 route committed.
	resp, err := s.buildReply(msg, dhcpv4.MessageTypeAck, reqIP)
	if err != nil {
		logger.Errorf(ctx, err, "build ACK for %s", mac)
		return
	}

	// Install the /32 host route before committing lease state. Without the
	// route the client would lease an IP that is not actually reachable via
	// the host, so a route failure must abort the ACK. RouteReplace is
	// idempotent, so a client re-REQUEST after a committed lease is safe.
	if err := addRoute(reqIP, s.linkIndex); err != nil {
		logger.Errorf(ctx, err, "add route %s; NAKing", reqIP)
		s.sendNAK(ctx, conn, peer, msg)
		return
	}

	// Commit: release any stale offer, promote the IP from free to leased.
	if oldIP := s.offers.remove(mac); oldIP != nil && !oldIP.Equal(reqIP) {
		s.pool.release(oldIP)
	}
	s.pool.markUsed(reqIP)
	s.leases.add(mac, reqIP, s.conf.LeaseTime)

	if _, err := conn.WriteTo(resp.ToBytes(), peer); err != nil {
		// Route + lease are now committed; the client will re-REQUEST and
		// hit the isLeasedTo branch above, which re-issues the ACK.
		logger.Errorf(ctx, err, "send ACK to %s (committed; awaiting client retry)", mac)
		return
	}

	if err := s.leases.save(); err != nil {
		logger.Error(ctx, err, "persist leases after ACK")
	}
	logger.Infof(ctx, "ACK %s -> %s", reqIP, mac)
}
