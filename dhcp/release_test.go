package dhcp

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReleaseLeaseReclaimsRouteAndPoolSlot(t *testing.T) {
	leasePath := filepath.Join(t.TempDir(), "leases.json")
	ip := net.ParseIP("10.0.0.10").To4()
	mac := mustMAC(t, "aa:bb:cc:dd:ee:01")
	srv := New(Config{LeaseFile: leasePath}, []net.IP{ip})
	if !srv.pool.tryClaim(ip) {
		t.Fatal("claim test IP")
	}
	srv.leases.add(mac, ip, time.Hour)

	orig := delRouteFn
	var deleted net.IP
	delRouteFn = func(got net.IP, _ int) error {
		deleted = got
		return nil
	}
	defer func() { delRouteFn = orig }()

	released, err := srv.ReleaseLease(t.Context(), mac)
	if err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
	if !released {
		t.Fatal("ReleaseLease reported no lease")
	}
	if !deleted.Equal(ip) {
		t.Errorf("deleted route = %s, want %s", deleted, ip)
	}
	if got := srv.pool.freeCount(); got != 1 {
		t.Errorf("free pool slots = %d, want 1", got)
	}
	if got := srv.leases.ipForMAC(mac); got != nil {
		t.Errorf("lease still present: %s", got)
	}
	data, err := os.ReadFile(leasePath)
	if err != nil {
		t.Fatalf("read persisted leases: %v", err)
	}
	var entries []leaseEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("decode persisted leases: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("persisted leases = %s, want empty", data)
	}

	released, err = srv.ReleaseLease(t.Context(), mac)
	if err != nil || released {
		t.Errorf("idempotent release = (%v, %v), want (false, nil)", released, err)
	}
}

func TestReleaseLeaseStillReclaimsWhenRouteDeleteFails(t *testing.T) {
	leasePath := filepath.Join(t.TempDir(), "leases.json")
	ip := net.ParseIP("10.0.0.10").To4()
	mac := mustMAC(t, "aa:bb:cc:dd:ee:01")
	srv := New(Config{LeaseFile: leasePath}, []net.IP{ip})
	if !srv.pool.tryClaim(ip) {
		t.Fatal("claim test IP")
	}
	srv.leases.add(mac, ip, time.Hour)

	orig := delRouteFn
	delRouteFn = func(net.IP, int) error { return errors.New("route gone") }
	defer func() { delRouteFn = orig }()

	released, err := srv.ReleaseLease(t.Context(), mac)
	if !released || err != nil {
		t.Fatalf("ReleaseLease = (%v, %v), want (true, nil)", released, err)
	}
	if got := srv.pool.freeCount(); got != 1 {
		t.Errorf("free pool slots = %d, want 1", got)
	}
	if got := srv.leases.ipForMAC(mac); got != nil {
		t.Errorf("lease still present: %s", got)
	}
}

func TestReleaseLeaseRollsBackWhenPersistenceFails(t *testing.T) {
	ip := net.ParseIP("10.0.0.10").To4()
	mac := mustMAC(t, "aa:bb:cc:dd:ee:01")
	srv := New(Config{LeaseFile: filepath.Join(t.TempDir(), "missing", "leases.json")}, []net.IP{ip})
	if !srv.pool.tryClaim(ip) {
		t.Fatal("claim test IP")
	}
	srv.leases.add(mac, ip, time.Hour)

	orig := delRouteFn
	deleteCalls := 0
	delRouteFn = func(net.IP, int) error {
		deleteCalls++
		return nil
	}
	defer func() { delRouteFn = orig }()

	released, err := srv.ReleaseLease(t.Context(), mac)
	if !released || err == nil {
		t.Fatalf("ReleaseLease = (%v, %v), want released with persistence error", released, err)
	}
	if deleteCalls != 0 {
		t.Errorf("route delete calls = %d, want 0 before persistence succeeds", deleteCalls)
	}
	if got := srv.pool.freeCount(); got != 0 {
		t.Errorf("free pool slots = %d, want 0 after rollback", got)
	}
	if got := srv.leases.ipForMAC(mac); !got.Equal(ip) {
		t.Errorf("restored lease = %s, want %s", got, ip)
	}
}
