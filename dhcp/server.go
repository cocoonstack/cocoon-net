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
	defaultLeaseTime     = 24 * time.Hour
	leaseCleanupInterval = time.Minute
	offerTimeout         = 60 * time.Second
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
	offers *pendingOffers

	linkIndex int // cached kernel interface index for route operations
	mu        sync.Mutex
	stopped   bool
}

// New creates a DHCP server. IPs are the allocatable pool (excluding gateway).
func New(conf Config, ips []net.IP) *Server {
	if conf.LeaseTime == 0 {
		conf.LeaseTime = defaultLeaseTime
	}
	return &Server{
		conf:   conf,
		pool:   newIPPool(ips),
		leases: newLeaseStore(conf.LeaseFile),
		offers: newPendingOffers(offerTimeout),
	}
}

// Run starts the DHCP server and blocks until ctx is canceled.
func (s *Server) Run(ctx context.Context) error {
	logger := log.WithFunc("dhcp.Run")

	linkIdx, resolveErr := resolveLinkIndex(s.conf.Interface)
	if resolveErr != nil {
		return fmt.Errorf("resolve interface %s: %w", s.conf.Interface, resolveErr)
	}
	s.linkIndex = linkIdx

	if err := s.leases.load(); err != nil {
		logger.Warnf(ctx, "load leases: %v (starting fresh)", err)
	} else {
		s.restoreLeases(ctx)
	}

	laddr := &net.UDPAddr{IP: net.IPv4zero, Port: dhcpv4.ServerPort}
	// Bind ctx to the packet handler via a closure so handlers don't need
	// to stash it on the Server struct.
	srv, err := server4.NewServer(s.conf.Interface, laddr,
		func(conn net.PacketConn, peer net.Addr, msg *dhcpv4.DHCPv4) {
			s.handler(ctx, conn, peer, msg)
		})
	if err != nil {
		return fmt.Errorf("create DHCP server: %w", err)
	}

	logger.Infof(ctx, "DHCP server listening on %s (pool: %d IPs, gateway: %s)",
		s.conf.Interface, s.pool.freeCount(), s.conf.Gateway)

	go s.cleanupLoop(ctx)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve() }()

	select {
	case <-ctx.Done():
		s.mu.Lock()
		s.stopped = true
		s.mu.Unlock()
		_ = srv.Close()
		if err := s.leases.save(); err != nil {
			logger.Errorf(ctx, err, "persist leases on shutdown")
		}
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

// handler dispatches each DHCP packet to a message-type-specific handler.
func (s *Server) handler(ctx context.Context, conn net.PacketConn, peer net.Addr, msg *dhcpv4.DHCPv4) {
	if msg.OpCode != dhcpv4.OpcodeBootRequest {
		return
	}

	mac := msg.ClientHWAddr

	switch msg.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		s.handleDiscover(ctx, conn, peer, msg, mac)
	case dhcpv4.MessageTypeRequest:
		s.handleRequest(ctx, conn, peer, msg, mac)
	case dhcpv4.MessageTypeRelease:
		s.handleRelease(ctx, mac)
	}
}
