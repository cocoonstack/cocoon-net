// Package dhcp implements the cocoon-net DHCP server: leases, the IP pool, and the request/release handlers.
package dhcp

import (
	"context"
	"errors"
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

// route ops are vars so tests can stub the netlink calls
var (
	addRouteFn = addRoute
	delRouteFn = delRoute
)

// Config holds DHCP server parameters.
type Config struct {
	Interface  string
	Gateway    net.IP
	SubnetMask net.IPMask
	DNSServers []net.IP
	LeaseTime  time.Duration
	LeaseFile  string
	// ControlSocket exposes the root-only local lease reclamation API. Empty disables it.
	ControlSocket string
}

// Server is an embedded DHCPv4 server that leases IPs from a fixed pool and keeps /32 host routes in step.
type Server struct {
	conf   Config
	pool   *ipPool
	leases *leaseStore
	offers *pendingOffers

	lifecycleMu sync.Mutex // serializes multi-structure lease, route, and pool transitions
	linkIndex   int        // cached kernel interface index for route operations
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

	control, err := newControlServer(s.conf.ControlSocket, s)
	if err != nil {
		return fmt.Errorf("start control server: %w", err)
	}
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	laddr := &net.UDPAddr{IP: net.IPv4zero, Port: dhcpv4.ServerPort}
	srv, err := server4.NewServer(s.conf.Interface, laddr,
		func(conn net.PacketConn, peer net.Addr, msg *dhcpv4.DHCPv4) {
			s.handler(ctx, conn, peer, msg)
		})
	if err != nil {
		return fmt.Errorf("create DHCP server: %w", err)
	}

	logger.Infof(ctx, "DHCP server listening on %s (pool: %d IPs, gateway: %s)",
		s.conf.Interface, s.pool.freeCount(), s.conf.Gateway)

	go s.cleanupLoop(runCtx)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve() }()
	var controlErrCh <-chan error
	if control != nil {
		controlErrCh = control.serve(runCtx)
	}

	select {
	case <-ctx.Done():
		_ = srv.Close()
		cancelRun()
		if control != nil {
			if err := <-controlErrCh; err != nil {
				return fmt.Errorf("stop control server: %w", err)
			}
		}
		// held until exit: a handler still queued must not start a transaction the process cannot finish
		s.lifecycleMu.Lock()
		logger.Info(ctx, "DHCP server stopped")
		return nil
	case err := <-errCh:
		cancelRun()
		return fmt.Errorf("DHCP server: %w", err)
	case err := <-controlErrCh:
		_ = srv.Close()
		cancelRun()
		if err == nil {
			if ctx.Err() != nil {
				logger.Info(ctx, "DHCP server stopped")
				return nil
			}
			return errors.New("control server stopped unexpectedly")
		}
		return fmt.Errorf("control server: %w", err)
	}
}

// PoolAvailable returns the number of unallocated pool IPs, read per metrics scrape.
func (s *Server) PoolAvailable() int { return s.pool.freeCount() }

// ActiveLeaseCount returns the number of unexpired leases, read per metrics scrape.
func (s *Server) ActiveLeaseCount() int { return s.leases.activeCount() }

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
