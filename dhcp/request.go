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

	// If this MAC already holds the IP via a pending offer or active
	// lease, the IP is in pool.used; otherwise tryClaim moves it from
	// free to used atomically. The atomic check-and-commit is what
	// guarantees two concurrent REQUESTs for the same free IP cannot
	// both succeed.
	alreadyHeld := s.offers.isOfferedTo(mac, reqIP) || s.leases.isLeasedTo(mac, reqIP)
	if !alreadyHeld && !s.pool.tryClaim(reqIP) {
		s.sendNAK(ctx, conn, peer, msg)
		logger.Infof(ctx, "NAK %s -> %s (not available)", reqIP, mac)
		return
	}

	// Build the reply. A buildReply failure here must release the claim
	// we just took so the IP doesn't leak out of the pool.
	resp, err := s.buildReply(msg, dhcpv4.MessageTypeAck, reqIP)
	if err != nil {
		if !alreadyHeld {
			s.pool.release(reqIP)
		}
		logger.Errorf(ctx, err, "build ACK for %s", mac)
		return
	}

	// Install the /32 host route before committing lease state. Without the
	// route the client would lease an IP that is not actually reachable via
	// the host, so a route failure must abort the ACK and release the
	// claim. RouteReplace is idempotent, so a client re-REQUEST after a
	// committed lease is safe.
	if err := addRoute(reqIP, s.linkIndex); err != nil {
		if !alreadyHeld {
			s.pool.release(reqIP)
		}
		logger.Errorf(ctx, err, "add route %s; NAKing", reqIP)
		s.sendNAK(ctx, conn, peer, msg)
		return
	}

	// Commit lease state. If the client moved from one offered IP to a
	// different requested IP, release the orphaned offer's IP back to
	// the pool.
	if oldIP := s.offers.remove(mac); oldIP != nil && !oldIP.Equal(reqIP) {
		s.pool.release(oldIP)
	}
	if evicted := s.leases.add(mac, reqIP, s.conf.LeaseTime); len(evicted) > 0 {
		logger.Warnf(ctx, "evicted stale lease(s) for %s held by %v while ACKing %s", reqIP, evicted, mac)
	}

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
