package dhcp

import (
	"net"
	"path/filepath"
	"testing"
	"time"
)

func mustMAC(t *testing.T, s string) net.HardwareAddr {
	t.Helper()
	mac, err := net.ParseMAC(s)
	if err != nil {
		t.Fatalf("parse mac %q: %v", s, err)
	}
	return mac
}

func TestLeaseStore_AddNoEviction(t *testing.T) {
	t.Parallel()

	s := newLeaseStore(filepath.Join(t.TempDir(), "leases.json"))
	mac := mustMAC(t, "aa:bb:cc:dd:ee:01")
	ip := net.ParseIP("10.0.0.10").To4()

	evicted := s.add(mac, ip, time.Hour)
	if len(evicted) != 0 {
		t.Fatalf("first add should not evict, got %d", len(evicted))
	}
	if got := s.ipForMAC(mac); !got.Equal(ip) {
		t.Errorf("ipForMAC=%s, want %s", got, ip)
	}
}

// TestLeaseStore_AddRebindSameMACDifferentIP exercises the case the
// finding flags: a MAC that previously had IP-A is now leasing IP-B.
// The store must surface the old IP-A so the caller can delRoute and
// pool.release it — otherwise the IP-A route + pool slot are stranded.
func TestLeaseStore_AddRebindSameMACDifferentIP(t *testing.T) {
	t.Parallel()

	s := newLeaseStore(filepath.Join(t.TempDir(), "leases.json"))
	mac := mustMAC(t, "aa:bb:cc:dd:ee:01")
	oldIP := net.ParseIP("10.0.0.10").To4()
	newIP := net.ParseIP("10.0.0.11").To4()

	s.add(mac, oldIP, time.Hour)
	evicted := s.add(mac, newIP, time.Hour)

	if len(evicted) != 1 {
		t.Fatalf("rebind should report 1 displaced lease, got %d", len(evicted))
	}
	if evicted[0].MAC != mac.String() {
		t.Errorf("evicted.MAC=%q, want %q", evicted[0].MAC, mac.String())
	}
	if !evicted[0].IP.Equal(oldIP) {
		t.Errorf("evicted.IP=%s, want %s", evicted[0].IP, oldIP)
	}
	if got := s.ipForMAC(mac); !got.Equal(newIP) {
		t.Errorf("ipForMAC after rebind=%s, want %s", got, newIP)
	}
}

// TestLeaseStore_AddSameMACSameIP is the renewal path — same MAC, same
// IP. No displacement should be reported.
func TestLeaseStore_AddSameMACSameIP(t *testing.T) {
	t.Parallel()

	s := newLeaseStore(filepath.Join(t.TempDir(), "leases.json"))
	mac := mustMAC(t, "aa:bb:cc:dd:ee:01")
	ip := net.ParseIP("10.0.0.10").To4()

	s.add(mac, ip, time.Hour)
	evicted := s.add(mac, ip, time.Hour)
	if len(evicted) != 0 {
		t.Fatalf("renewal should not evict, got %d", len(evicted))
	}
}

// TestLeaseStore_AddOtherMACSameIP is the TOCTOU-leftover path —
// another MAC holds the same IP. The other MAC's lease must be
// dropped and surfaced for logging.
func TestLeaseStore_AddOtherMACSameIP(t *testing.T) {
	t.Parallel()

	s := newLeaseStore(filepath.Join(t.TempDir(), "leases.json"))
	macA := mustMAC(t, "aa:bb:cc:dd:ee:01")
	macB := mustMAC(t, "aa:bb:cc:dd:ee:02")
	ip := net.ParseIP("10.0.0.10").To4()

	s.add(macA, ip, time.Hour)
	evicted := s.add(macB, ip, time.Hour)

	if len(evicted) != 1 {
		t.Fatalf("conflicting add should evict the other MAC, got %d", len(evicted))
	}
	if evicted[0].MAC != macA.String() {
		t.Errorf("evicted.MAC=%q, want %q", evicted[0].MAC, macA.String())
	}
	if !evicted[0].IP.Equal(ip) {
		t.Errorf("evicted.IP=%s, want %s", evicted[0].IP, ip)
	}
	// macA's lease must be gone; macB now owns the IP.
	if got := s.ipForMAC(macA); got != nil {
		t.Errorf("macA should have no lease, got %s", got)
	}
	if got := s.ipForMAC(macB); !got.Equal(ip) {
		t.Errorf("macB ipForMAC=%s, want %s", got, ip)
	}
}

// TestLeaseStore_AddOtherMACSameIPExpired: an expired other-MAC entry
// for the same IP must NOT be reported as eviction (it's already
// effectively gone), to avoid spurious delRoute traffic from the
// caller.
func TestLeaseStore_AddOtherMACSameIPExpired(t *testing.T) {
	t.Parallel()

	s := newLeaseStore(filepath.Join(t.TempDir(), "leases.json"))
	macA := mustMAC(t, "aa:bb:cc:dd:ee:01")
	macB := mustMAC(t, "aa:bb:cc:dd:ee:02")
	ip := net.ParseIP("10.0.0.10").To4()

	// Force an expired entry for macA.
	s.add(macA, ip, -time.Hour)

	evicted := s.add(macB, ip, time.Hour)
	if len(evicted) != 0 {
		t.Fatalf("expired other-MAC entry should not be reported, got %d", len(evicted))
	}
}

func TestLeaseStore_SaveLoadRoundtrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "leases.json")
	src := newLeaseStore(path)
	mac := mustMAC(t, "aa:bb:cc:dd:ee:01")
	ip := net.ParseIP("10.0.0.10").To4()
	src.add(mac, ip, time.Hour)
	if err := src.save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	dst := newLeaseStore(path)
	if err := dst.load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := dst.ipForMAC(mac); !got.Equal(ip) {
		t.Errorf("ipForMAC after reload=%s, want %s", got, ip)
	}
}
