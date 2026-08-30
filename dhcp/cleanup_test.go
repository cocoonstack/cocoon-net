package dhcp

import (
	"net"
	"testing"
	"time"
)

func TestRestoreLeasesDropsIPOutsidePool(t *testing.T) {
	orig := addRouteFn
	addRouteFn = func(net.IP, int) error { return nil }
	defer func() { addRouteFn = orig }()

	srv, conn, _ := newTestServer(t)
	defer conn.Close()
	inside := net.ParseIP("10.0.0.10").To4()
	foreign := net.ParseIP("172.16.5.5").To4()
	macIn := mustMAC(t, "aa:bb:cc:dd:ee:01")
	macOut := mustMAC(t, "aa:bb:cc:dd:ee:02")
	srv.leases.add(macIn, inside, time.Hour)
	srv.leases.add(macOut, foreign, time.Hour)

	srv.restoreLeases(t.Context())

	if got := srv.leases.ipForMAC(macOut); got != nil {
		t.Fatalf("lease outside the pool kept: %s", got)
	}
	if got := srv.leases.ipForMAC(macIn); !got.Equal(inside) {
		t.Fatalf("lease inside the pool lost: %v", got)
	}
	if srv.pool.tryClaim(foreign) {
		t.Fatal("IP outside the pool became allocatable")
	}
	if got := srv.pool.freeCount(); got != 1 {
		t.Fatalf("free = %d, want 1", got)
	}
}
