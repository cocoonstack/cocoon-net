package dhcp

import (
	"net"
	"sync"

	"github.com/cocoonstack/cocoon-net/platform"
)

// ipPool tracks which IPs from the fixed pool are free or in use.
type ipPool struct {
	mu   sync.RWMutex
	free map[uint32]net.IP // IPs not yet leased
	used map[uint32]bool   // currently leased IPs
}

func newIPPool(ips []net.IP) *ipPool {
	free := make(map[uint32]net.IP, len(ips))
	for _, ip := range ips {
		free[ipKey(ip)] = ip.To4()
	}
	return &ipPool{
		free: free,
		used: make(map[uint32]bool),
	}
}

// allocate returns an available IP and removes it from the free set.
func (p *ipPool) allocate() net.IP {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, ip := range p.free {
		delete(p.free, k)
		p.used[k] = true
		return ip
	}
	return nil
}

// release returns an IP to the free pool.
func (p *ipPool) release(ip net.IP) {
	if ip == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	k := ipKey(ip)
	delete(p.used, k)
	p.free[k] = ip.To4()
}

// markUsed moves an IP from free to used (for lease restoration).
func (p *ipPool) markUsed(ip net.IP) {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := ipKey(ip)
	delete(p.free, k)
	p.used[k] = true
}

// isFree checks if an IP is in the free (unallocated) set.
func (p *ipPool) isFree(ip net.IP) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.free[ipKey(ip)]
	return ok
}

// freeCount returns the number of unallocated IPs.
func (p *ipPool) freeCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.free)
}

// ipKey normalises an IPv4 net.IP (4-byte or 16-byte form) to its uint32
// representation for use as a map key. Delegates to platform.IP4ToUint32
// to keep a single source of truth.
func ipKey(ip net.IP) uint32 {
	if v4 := ip.To4(); v4 != nil {
		return platform.IP4ToUint32(v4.String())
	}
	return 0
}
