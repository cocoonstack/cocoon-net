package dhcp

import (
	"net"
	"sync"
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
		free[ipToUint32(ip)] = ip.To4()
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
	k := ipToUint32(ip.To4())
	delete(p.used, k)
	p.free[k] = ip.To4()
}

// markUsed moves an IP from free to used (for lease restoration).
func (p *ipPool) markUsed(ip net.IP) {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := ipToUint32(ip.To4())
	delete(p.free, k)
	p.used[k] = true
}

// isFree checks if an IP is in the free (unallocated) set.
func (p *ipPool) isFree(ip net.IP) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.free[ipToUint32(ip.To4())]
	return ok
}

// freeCount returns the number of unallocated IPs.
func (p *ipPool) freeCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.free)
}

func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3]) //nolint:mnd
}
