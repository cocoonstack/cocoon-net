package dhcp

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

func TestReplyPeerBroadcastsToAnUnspecifiedSource(t *testing.T) {
	for _, tc := range []struct {
		name string
		peer net.Addr
		want string
	}{
		{"unspecified", &net.UDPAddr{IP: net.IPv4zero, Port: 68}, "255.255.255.255:68"},
		{"nil ip", &net.UDPAddr{Port: 68}, "255.255.255.255:68"},
		{"addressed", &net.UDPAddr{IP: net.ParseIP("10.0.0.5").To4(), Port: 68}, "10.0.0.5:68"},
	} {
		if got := replyPeer(tc.peer).String(); got != tc.want {
			t.Errorf("%s: replyPeer = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestServeDispatchesParsedPacketsAndSkipsMalformedOnes(t *testing.T) {
	srv, conn, _ := newTestServer(t)
	client, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen client: %v", err)
	}
	defer client.Close()

	done := make(chan error, 1)
	go func() { done <- srv.serve(context.Background(), conn) }()
	if _, err := client.WriteTo([]byte("not a dhcp packet"), conn.LocalAddr()); err != nil {
		t.Fatalf("send garbage: %v", err)
	}
	discover, err := dhcpv4.New(dhcpv4.WithMessageType(dhcpv4.MessageTypeDiscover))
	if err != nil {
		t.Fatalf("build discover: %v", err)
	}
	discover.ClientHWAddr = mustMAC(t, "aa:bb:cc:dd:ee:01")
	if _, err := client.WriteTo(discover.ToBytes(), conn.LocalAddr()); err != nil {
		t.Fatalf("send discover: %v", err)
	}

	if err := client.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, datagramSize)
	n, _, err := client.ReadFrom(buf)
	if err != nil {
		t.Fatalf("no reply to the DISCOVER that followed a malformed packet: %v", err)
	}
	reply, err := dhcpv4.FromBytes(buf[:n])
	if err != nil {
		t.Fatalf("parse reply: %v", err)
	}
	if reply.MessageType() != dhcpv4.MessageTypeOffer {
		t.Fatalf("reply = %s, want OFFER", reply.MessageType())
	}

	conn.Close()
	if err := <-done; err == nil {
		t.Fatal("serve returned nil after the conn closed")
	}
}
