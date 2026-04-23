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

	// Commit: move from pending/free to leased.
	// Release the offered IP back to pool if client requested a different one.
	if oldIP := s.offers.remove(mac); oldIP != nil && !oldIP.Equal(reqIP) {
		s.pool.release(oldIP)
	}
	s.pool.markUsed(reqIP)
	s.leases.add(mac, reqIP, s.conf.LeaseTime)

	if err := addRoute(reqIP, s.linkIndex); err != nil {
		logger.Errorf(ctx, err, "add route %s", reqIP)
	}

	resp, err := s.buildReply(msg, dhcpv4.MessageTypeAck, reqIP)
	if err != nil {
		logger.Errorf(ctx, err, "build ACK for %s", mac)
		return
	}

	if _, err := conn.WriteTo(resp.ToBytes(), peer); err != nil {
		logger.Errorf(ctx, err, "send ACK to %s", mac)
		return
	}

	if err := s.leases.save(); err != nil {
		logger.Errorf(ctx, err, "persist leases after ACK")
	}
	logger.Infof(ctx, "ACK %s -> %s", reqIP, mac)
}
