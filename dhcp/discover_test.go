package dhcp

import (
	"net"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

func TestHandleDiscoverRefreshesOffer(t *testing.T) {
	t.Parallel()

	srv, conn, peer := newTestServer(t)
	defer conn.Close()
	mac := mustMAC(t, "aa:bb:cc:dd:ee:01")
	msg := discoverMsg(t, mac)

	srv.handleDiscover(t.Context(), conn, peer, msg, mac)
	offered := srv.offers.offers[mac.String()]
	offered.Expiry = time.Now().Add(time.Second)

	srv.handleDiscover(t.Context(), conn, peer, msg, mac)
	refreshed := srv.offers.offers[mac.String()]
	if !refreshed.IP.Equal(offered.IP) {
		t.Fatalf("re-DISCOVER changed the offered IP: %s -> %s", offered.IP, refreshed.IP)
	}
	if !refreshed.Expiry.After(time.Now().Add(30 * time.Second)) {
		t.Fatalf("re-DISCOVER did not refresh the offer, expiry %s", refreshed.Expiry)
	}
	if got := srv.pool.freeCount(); got != 1 {
		t.Fatalf("free = %d, want 1 (no double allocation)", got)
	}
}

func discoverMsg(t *testing.T, mac net.HardwareAddr) *dhcpv4.DHCPv4 {
	t.Helper()
	msg, err := dhcpv4.New(dhcpv4.WithMessageType(dhcpv4.MessageTypeDiscover))
	if err != nil {
		t.Fatalf("build discover: %v", err)
	}
	msg.ClientHWAddr = mac
	return msg
}
