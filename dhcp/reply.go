package dhcp

import (
	"context"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/projecteru2/core/log"
)

// buildReply constructs a DHCP reply with standard options.
func (s *Server) buildReply(req *dhcpv4.DHCPv4, msgType dhcpv4.MessageType, ip net.IP) (*dhcpv4.DHCPv4, error) {
	return dhcpv4.NewReplyFromRequest(req,
		dhcpv4.WithMessageType(msgType),
		dhcpv4.WithYourIP(ip),
		dhcpv4.WithServerIP(s.conf.Gateway),
		dhcpv4.WithOption(dhcpv4.OptSubnetMask(s.conf.SubnetMask)),
		dhcpv4.WithOption(dhcpv4.OptRouter(s.conf.Gateway)),
		dhcpv4.WithOption(dhcpv4.OptDNS(s.conf.DNSServers...)),
		dhcpv4.WithOption(dhcpv4.OptIPAddressLeaseTime(s.conf.LeaseTime)),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(s.conf.Gateway)),
	)
}

func (s *Server) sendNAK(ctx context.Context, conn net.PacketConn, peer net.Addr, msg *dhcpv4.DHCPv4) {
	logger := log.WithFunc("dhcp.sendNAK")
	resp, err := dhcpv4.NewReplyFromRequest(msg,
		dhcpv4.WithMessageType(dhcpv4.MessageTypeNak),
		dhcpv4.WithServerIP(s.conf.Gateway),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(s.conf.Gateway)),
	)
	if err != nil {
		logger.Error(ctx, err, "build NAK")
		return
	}
	if _, err := conn.WriteTo(resp.ToBytes(), peer); err != nil {
		logger.Error(ctx, err, "send NAK")
	}
}
