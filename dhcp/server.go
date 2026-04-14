package dhcp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
	"github.com/projecteru2/core/log"
)

const (
	defaultLeaseTime = 24 * time.Hour
	leaseCleanupInterval = time.Minute
)

// Config holds DHCP server parameters.
type Config struct {
	Interface  string
	Gateway    net.IP
	SubnetMask net.IPMask
	DNSServers []net.IP
	LeaseTime  time.Duration
	LeaseFile  string
}

// Server is an embedded DHCPv4 server that allocates IPs from a fixed pool,
// manages leases, and adds/removes /32 host routes on lease events.
type Server struct {
	conf   Config
	pool   *ipPool
	leases *leaseStore
	srv    *server4.Server

	mu      sync.Mutex
	stopped bool
}

// New creates a DHCP server. IPs are the allocatable pool (excluding gateway).
func New(conf Config, ips []net.IP) *Server {
	if conf.LeaseTime == 0 {
		conf.LeaseTime = defaultLeaseTime
	}
	return &Server{
		conf:   conf,
		pool:   newIPPool(ips),
		leases: newLeaseStore(conf.LeaseFile, conf.Interface),
	}
}

// Run starts the DHCP server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	logger := log.WithFunc("dhcp.Run")

	if err := s.leases.load(); err != nil {
		logger.Warnf(ctx, "load leases: %v (starting fresh)", err)
	} else {
		s.restoreLeases(ctx)
	}

	laddr := &net.UDPAddr{IP: net.IPv4zero, Port: dhcpv4.ServerPort}
	srv, err := server4.NewServer(s.conf.Interface, laddr, s.handler)
	if err != nil {
		return fmt.Errorf("create DHCP server: %w", err)
	}
	s.srv = srv

	logger.Infof(ctx, "DHCP server listening on %s (pool: %d IPs, gateway: %s)",
		s.conf.Interface, s.pool.freeCount(), s.conf.Gateway)

	// Lease cleanup goroutine.
	go s.cleanupLoop(ctx)

	// Serve blocks until error or close.
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve() }()

	select {
	case <-ctx.Done():
		s.mu.Lock()
		s.stopped = true
		s.mu.Unlock()
		srv.Close()
		_ = s.leases.save()
		logger.Info(ctx, "DHCP server stopped")
		return nil
	case err := <-errCh:
		s.mu.Lock()
		stopped := s.stopped
		s.mu.Unlock()
		if stopped {
			return nil
		}
		return fmt.Errorf("DHCP server: %w", err)
	}
}

// handler processes each DHCP packet.
func (s *Server) handler(conn net.PacketConn, peer net.Addr, msg *dhcpv4.DHCPv4) {
	ctx := context.Background()
	logger := log.WithFunc("dhcp.handler")

	if msg.OpCode != dhcpv4.OpcodeBootRequest {
		return
	}

	mac := msg.ClientHWAddr
	msgType := msg.MessageType()

	switch msgType {
	case dhcpv4.MessageTypeDiscover:
		s.handleDiscover(ctx, conn, peer, msg, mac, logger)
	case dhcpv4.MessageTypeRequest:
		s.handleRequest(ctx, conn, peer, msg, mac, logger)
	case dhcpv4.MessageTypeRelease:
		s.handleRelease(ctx, msg, mac, logger)
	}
}

func (s *Server) handleDiscover(ctx context.Context, conn net.PacketConn, peer net.Addr, msg *dhcpv4.DHCPv4, mac net.HardwareAddr, logger *log.Fields) {
	// Check existing lease first.
	ip := s.leases.ipForMAC(mac)
	if ip == nil {
		ip = s.pool.allocate()
		if ip == nil {
			logger.Warnf(ctx, "DISCOVER from %s: pool exhausted", mac)
			return
		}
	}

	resp, err := dhcpv4.NewReplyFromRequest(msg,
		dhcpv4.WithMessageType(dhcpv4.MessageTypeOffer),
		dhcpv4.WithYourIP(ip),
		dhcpv4.WithServerIP(s.conf.Gateway),
		dhcpv4.WithOption(dhcpv4.OptSubnetMask(s.conf.SubnetMask)),
		dhcpv4.WithOption(dhcpv4.OptRouter(s.conf.Gateway)),
		dhcpv4.WithOption(dhcpv4.OptDNS(s.conf.DNSServers...)),
		dhcpv4.WithOption(dhcpv4.OptIPAddressLeaseTime(s.conf.LeaseTime)),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(s.conf.Gateway)),
	)
	if err != nil {
		logger.Warnf(ctx, "build OFFER for %s: %v", mac, err)
		return
	}

	if _, err := conn.WriteTo(resp.ToBytes(), peer); err != nil {
		logger.Warnf(ctx, "send OFFER to %s: %v", mac, err)
		return
	}
	logger.Infof(ctx, "OFFER %s → %s", ip, mac)
}

func (s *Server) handleRequest(ctx context.Context, conn net.PacketConn, peer net.Addr, msg *dhcpv4.DHCPv4, mac net.HardwareAddr, logger *log.Fields) {
	reqIP := msg.RequestedIPAddress()
	if reqIP == nil || reqIP.IsUnspecified() {
		reqIP = msg.ClientIPAddr
	}
	if reqIP == nil || reqIP.IsUnspecified() {
		logger.Warnf(ctx, "REQUEST from %s: no IP requested", mac)
		return
	}

	// Validate the requested IP is in our pool or already leased to this MAC.
	if !s.pool.contains(reqIP) && !s.leases.isLeasedTo(mac, reqIP) {
		s.sendNAK(conn, peer, msg, logger)
		logger.Warnf(ctx, "NAK %s → %s (not in pool)", reqIP, mac)
		return
	}

	// Commit lease.
	s.pool.markUsed(reqIP)
	s.leases.add(mac, reqIP, s.conf.LeaseTime)

	// Add /32 host route.
	if err := addRoute(reqIP, s.conf.Interface); err != nil {
		logger.Warnf(ctx, "add route %s: %v", reqIP, err)
	}

	resp, err := dhcpv4.NewReplyFromRequest(msg,
		dhcpv4.WithMessageType(dhcpv4.MessageTypeAck),
		dhcpv4.WithYourIP(reqIP),
		dhcpv4.WithServerIP(s.conf.Gateway),
		dhcpv4.WithOption(dhcpv4.OptSubnetMask(s.conf.SubnetMask)),
		dhcpv4.WithOption(dhcpv4.OptRouter(s.conf.Gateway)),
		dhcpv4.WithOption(dhcpv4.OptDNS(s.conf.DNSServers...)),
		dhcpv4.WithOption(dhcpv4.OptIPAddressLeaseTime(s.conf.LeaseTime)),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(s.conf.Gateway)),
	)
	if err != nil {
		logger.Warnf(ctx, "build ACK for %s: %v", mac, err)
		return
	}

	if _, err := conn.WriteTo(resp.ToBytes(), peer); err != nil {
		logger.Warnf(ctx, "send ACK to %s: %v", mac, err)
		return
	}

	_ = s.leases.save()
	logger.Infof(ctx, "ACK %s → %s", reqIP, mac)
}

func (s *Server) handleRelease(ctx context.Context, _ *dhcpv4.DHCPv4, mac net.HardwareAddr, logger *log.Fields) {
	ip := s.leases.ipForMAC(mac)
	if ip == nil {
		return
	}

	s.leases.remove(mac)
	s.pool.release(ip)

	if err := delRoute(ip, s.conf.Interface); err != nil {
		logger.Warnf(ctx, "del route %s: %v", ip, err)
	}

	_ = s.leases.save()
	logger.Infof(ctx, "RELEASE %s ← %s", ip, mac)
}

func (s *Server) sendNAK(conn net.PacketConn, peer net.Addr, msg *dhcpv4.DHCPv4, logger *log.Fields) {
	resp, err := dhcpv4.NewReplyFromRequest(msg,
		dhcpv4.WithMessageType(dhcpv4.MessageTypeNak),
		dhcpv4.WithServerIP(s.conf.Gateway),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(s.conf.Gateway)),
	)
	if err != nil {
		return
	}
	if _, err := conn.WriteTo(resp.ToBytes(), peer); err != nil {
		logger.Warnf(context.Background(), "send NAK: %v", err)
	}
}

// restoreLeases re-adds /32 routes for all non-expired leases on startup.
func (s *Server) restoreLeases(ctx context.Context) {
	logger := log.WithFunc("dhcp.restoreLeases")
	active := s.leases.activeLeases()
	for _, l := range active {
		s.pool.markUsed(l.IP)
		if err := addRoute(l.IP, s.conf.Interface); err != nil {
			logger.Warnf(ctx, "restore route %s: %v", l.IP, err)
		}
	}
	if len(active) > 0 {
		logger.Infof(ctx, "restored %d active leases", len(active))
	}
}

// cleanupLoop periodically removes expired leases and their routes.
func (s *Server) cleanupLoop(ctx context.Context) {
	logger := log.WithFunc("dhcp.cleanup")
	ticker := time.NewTicker(leaseCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			expired := s.leases.expireOld()
			for _, l := range expired {
				s.pool.release(l.IP)
				if err := delRoute(l.IP, s.conf.Interface); err != nil {
					logger.Warnf(ctx, "del expired route %s: %v", l.IP, err)
				}
				logger.Infof(ctx, "expired lease %s ← %s", l.IP, l.MAC)
			}
			if len(expired) > 0 {
				_ = s.leases.save()
			}
		}
	}
}
