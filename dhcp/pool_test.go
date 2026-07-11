package dhcp

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
)

func TestIPPool_TryClaimAtomic(t *testing.T) {
	t.Parallel()

	pool := newIPPool(parseIPs(t, "10.0.0.10", "10.0.0.11", "10.0.0.12"))
	ip := net.ParseIP("10.0.0.10").To4()

	if !pool.tryClaim(ip) {
		t.Fatalf("first tryClaim should succeed")
	}
	if pool.tryClaim(ip) {
		t.Fatalf("second tryClaim should fail")
	}
	pool.release(ip)
	if !pool.tryClaim(ip) {
		t.Fatalf("tryClaim after release should succeed")
	}
}

func TestIPPool_TryClaimNil(t *testing.T) {
	t.Parallel()

	pool := newIPPool(parseIPs(t, "10.0.0.10"))
	if pool.tryClaim(nil) {
		t.Fatalf("tryClaim(nil) must return false")
	}
}

func TestIPPool_TryClaimUnknownIP(t *testing.T) {
	t.Parallel()

	pool := newIPPool(parseIPs(t, "10.0.0.10"))
	if pool.tryClaim(net.ParseIP("10.0.0.99")) {
		t.Fatalf("tryClaim of out-of-pool IP must return false")
	}
}

// Many goroutines race for the same free IP; exactly one must win.
func TestIPPool_TryClaimRace(t *testing.T) {
	t.Parallel()

	const goroutines = 64
	pool := newIPPool(parseIPs(t, "10.0.0.10"))
	ip := net.ParseIP("10.0.0.10").To4()

	var (
		wins  atomic.Int32
		start sync.WaitGroup
		done  sync.WaitGroup
	)
	start.Add(1)
	for range goroutines {
		done.Go(func() {
			start.Wait()
			if pool.tryClaim(ip) {
				wins.Add(1)
			}
		})
	}
	start.Done()
	done.Wait()

	if got := wins.Load(); got != 1 {
		t.Fatalf("exactly one goroutine should claim the IP, got %d", got)
	}
	if pool.freeCount() != 0 {
		t.Fatalf("pool should be empty after race, got freeCount=%d", pool.freeCount())
	}
}

// Wider race: many goroutines target one IP inside a multi-IP pool.
// Guards against tryClaim wrongly claiming a neighbour entry.
func TestIPPool_TryClaimRaceManyIPs(t *testing.T) {
	t.Parallel()

	pool := newIPPool(parseIPs(t, "10.0.0.10", "10.0.0.11", "10.0.0.12", "10.0.0.13"))
	ip := net.ParseIP("10.0.0.12").To4()

	var (
		wins  atomic.Int32
		start sync.WaitGroup
		done  sync.WaitGroup
	)
	start.Add(1)
	for range 32 {
		done.Go(func() {
			start.Wait()
			if pool.tryClaim(ip) {
				wins.Add(1)
			}
		})
	}
	start.Done()
	done.Wait()

	if got := wins.Load(); got != 1 {
		t.Fatalf("exactly one goroutine should claim 10.0.0.12, got %d", got)
	}
	if pool.freeCount() != 3 {
		t.Fatalf("only one IP should have been claimed, freeCount=%d", pool.freeCount())
	}
}

func TestIPPool_MarkUsedAllocate(t *testing.T) {
	t.Parallel()

	pool := newIPPool(parseIPs(t, "10.0.0.10", "10.0.0.11"))
	pool.markUsed(net.ParseIP("10.0.0.10"))
	if pool.freeCount() != 1 {
		t.Fatalf("expect 1 free IP after markUsed, got %d", pool.freeCount())
	}
	ip := pool.allocate()
	if !ip.Equal(net.ParseIP("10.0.0.11")) {
		t.Fatalf("allocate returned %s, want 10.0.0.11", ip)
	}
	if pool.allocate() != nil {
		t.Fatalf("allocate on exhausted pool must return nil")
	}
}

func parseIPs(t *testing.T, ips ...string) []net.IP {
	t.Helper()
	out := make([]net.IP, 0, len(ips))
	for _, s := range ips {
		ip := net.ParseIP(s).To4()
		if ip == nil {
			t.Fatalf("parse %q", s)
		}
		out = append(out, ip)
	}
	return out
}
