package dhcp

import (
	"encoding/binary"
	"net"
	"sync"
)

// ipPool tracks which IPs from the fixed pool are free or in use. Map
// keys are the 4-byte IPv4 packed into a uint32 — fast hashing, no
// string conversion. Callers always pass net.IP via tryClaim/release/etc.
type ipPool struct {
	mu   sync.RWMutex
	free map[uint32]net.IP   // IPs not yet leased
	used map[uint32]struct{} // currently leased IPs (set semantics)
}

func newIPPool(ips []net.IP) *ipPool {
	free := make(map[uint32]net.IP, len(ips))
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			free[ipKey(v4)] = v4
		}
	}
	return &ipPool{
		free: free,
		used: make(map[uint32]struct{}),
	}
}

// allocate returns an available IP and removes it from the free set.
func (p *ipPool) allocate() net.IP {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, ip := range p.free {
		delete(p.free, k)
		p.used[k] = struct{}{}
		return ip
	}
	return nil
}

// release returns an IP to the free pool.
func (p *ipPool) release(ip net.IP) {
	v4 := ip.To4()
	if v4 == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	k := ipKey(v4)
	delete(p.used, k)
	p.free[k] = v4
}

// markUsed moves an IP from free to used (for lease restoration).
func (p *ipPool) markUsed(ip net.IP) {
	v4 := ip.To4()
	if v4 == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	k := ipKey(v4)
	delete(p.free, k)
	p.used[k] = struct{}{}
}

// tryClaim atomically moves ip from free to used and returns true.
// Returns false if ip is not in the free set (already claimed, or not
// part of this pool). The whole check-and-commit happens under p.mu so
// two concurrent REQUESTs racing for the same free IP can never both
// win — exactly one observes the IP in free and removes it.
func (p *ipPool) tryClaim(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	k := ipKey(v4)
	if _, free := p.free[k]; !free {
		return false
	}
	delete(p.free, k)
	p.used[k] = struct{}{}
	return true
}

// freeCount returns the number of unallocated IPs.
func (p *ipPool) freeCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.free)
}

// ipKey packs a 4-byte IPv4 into a uint32 map key. Callers must pass
// the 4-byte form (use ip.To4()) — passing a 16-byte form would read
// past the address bytes and produce a wrong key.
func ipKey(v4 net.IP) uint32 {
	return binary.BigEndian.Uint32(v4)
}
