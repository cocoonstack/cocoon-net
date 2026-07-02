package dhcp

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/cocoonstack/cocoon-net/metrics"
)

// These tests share the global DHCPLeaseTotal counter and the addRouteFn stub,
// so they run sequentially (no t.Parallel) and assert on before/after deltas.

func TestHandleRequestRecordsOK(t *testing.T) {
	srv, conn, peer := newTestServer(t)
	defer conn.Close()

	// Stub the netlink route add so the ACK path is reachable off-Linux.
	orig := addRouteFn
	addRouteFn = func(net.IP, int) error { return nil }
	defer func() { addRouteFn = orig }()

	mac := mustMAC(t, "aa:bb:cc:dd:ee:01")
	reqIP := net.ParseIP("10.0.0.10").To4()

	okBefore, failBefore := leaseCounts()
	srv.handleRequest(t.Context(), conn, peer, requestMsg(t, mac, reqIP), mac)
	okAfter, failAfter := leaseCounts()

	if okAfter-okBefore != 1 || failAfter != failBefore {
		t.Errorf("ok grant: ok delta=%v failed delta=%v, want 1 and 0", okAfter-okBefore, failAfter-failBefore)
	}
	if got := srv.leases.ipForMAC(mac); !got.Equal(reqIP) {
		t.Errorf("lease not committed: ipForMAC=%v", got)
	}
}

func TestHandleRequestRecordsFailed(t *testing.T) {
	srv, conn, peer := newTestServer(t)
	defer conn.Close()

	mac := mustMAC(t, "aa:bb:cc:dd:ee:01")
	reqIP := net.ParseIP("10.0.0.10").To4()
	srv.leases.add(mustMAC(t, "aa:bb:cc:dd:ee:99"), reqIP, time.Hour) // held by another MAC -> NAK

	okBefore, failBefore := leaseCounts()
	srv.handleRequest(t.Context(), conn, peer, requestMsg(t, mac, reqIP), mac)
	okAfter, failAfter := leaseCounts()

	if failAfter-failBefore != 1 || okAfter != okBefore {
		t.Errorf("nak: failed delta=%v ok delta=%v, want 1 and 0", failAfter-failBefore, okAfter-okBefore)
	}
}

func TestHandleRequestNoIPNotCounted(t *testing.T) {
	srv, conn, peer := newTestServer(t)
	defer conn.Close()

	mac := mustMAC(t, "aa:bb:cc:dd:ee:01")
	msg, err := dhcpv4.New(dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	msg.ClientHWAddr = mac

	okBefore, failBefore := leaseCounts()
	srv.handleRequest(t.Context(), conn, peer, msg, mac)
	okAfter, failAfter := leaseCounts()

	if okAfter != okBefore || failAfter != failBefore {
		t.Errorf("request without an IP must not record an outcome (ok %v->%v, failed %v->%v)", okBefore, okAfter, failBefore, failAfter)
	}
}

func newTestServer(t *testing.T) (*Server, net.PacketConn, net.Addr) {
	t.Helper()
	srv := New(Config{
		Interface:  "test0",
		Gateway:    net.ParseIP("10.0.0.1").To4(),
		SubnetMask: net.CIDRMask(24, 32),
		DNSServers: []net.IP{net.ParseIP("8.8.8.8").To4()},
		LeaseTime:  time.Hour,
		LeaseFile:  filepath.Join(t.TempDir(), "leases.json"),
	}, parseIPs(t, "10.0.0.10", "10.0.0.11"))

	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen packet: %v", err)
	}
	peer, err := net.ResolveUDPAddr("udp4", "127.0.0.1:9999")
	if err != nil {
		t.Fatalf("resolve peer: %v", err)
	}
	return srv, conn, peer
}

func requestMsg(t *testing.T, mac net.HardwareAddr, reqIP net.IP) *dhcpv4.DHCPv4 {
	t.Helper()
	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(reqIP)),
	)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	msg.ClientHWAddr = mac
	return msg
}

func leaseCounts() (ok, failed float64) {
	return testutil.ToFloat64(metrics.DHCPLeaseTotal.WithLabelValues("ok")),
		testutil.ToFloat64(metrics.DHCPLeaseTotal.WithLabelValues("failed"))
}
