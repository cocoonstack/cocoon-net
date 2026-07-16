package dhcp

import (
	"encoding/binary"
	"net"
	"sync"
)

// ipPool tracks which IPs are free or in use. Keys are the 4-byte
// IPv4 packed into a uint32.
type ipPool struct {
	mu   sync.RWMutex
	free map[uint32]net.IP
	used map[uint32]struct{}
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

func (p *ipPool) release(ip net.IP) {
	v4 := ip.To4()
	p.mu.Lock()
	defer p.mu.Unlock()
	k := ipKey(v4)
	delete(p.used, k)
	p.free[k] = v4
}

func (p *ipPool) markUsed(ip net.IP) {
	v4 := ip.To4()
	p.mu.Lock()
	defer p.mu.Unlock()
	k := ipKey(v4)
	delete(p.free, k)
	p.used[k] = struct{}{}
}

// tryClaim atomically moves ip from free to used under p.mu, so two
// concurrent REQUESTs for the same free IP cannot both win. Returns
// false if ip is not currently free (already claimed or not in pool).
func (p *ipPool) tryClaim(ip net.IP) bool {
	v4 := ip.To4()
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

func (p *ipPool) freeCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.free)
}

// ipKey packs a 4-byte IPv4 into a uint32 map key. Callers must pass
// the 4-byte form (use ip.To4()); a 16-byte net.IP reads from the wrong
// offset and yields a junk key.
func ipKey(v4 net.IP) uint32 {
	return binary.BigEndian.Uint32(v4)
}
