package dhcp

import (
	"net"
	"sync"
	"time"
)

// pendingOffer tracks an IP offered to a MAC that hasn't been committed yet.
type pendingOffer struct {
	IP     net.IP
	Expiry time.Time
}

// pendingOffers manages IPs that have been OFFERed but not yet ACKed.
// If the client never sends REQUEST, the offer expires and the IP
// is returned to the pool by the cleanup loop.
type pendingOffers struct {
	mu      sync.RWMutex
	offers  map[string]*pendingOffer // keyed by MAC string
	timeout time.Duration
}

func newPendingOffers(timeout time.Duration) *pendingOffers {
	return &pendingOffers{
		offers:  make(map[string]*pendingOffer),
		timeout: timeout,
	}
}

// add records a pending offer for mac. If this MAC already has a pending
// offer for a different IP, the old IP is returned so the caller can
// release it back to the pool.
func (p *pendingOffers) add(mac net.HardwareAddr, ip net.IP) net.IP {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := mac.String()
	var oldIP net.IP
	if old, ok := p.offers[key]; ok && !old.IP.Equal(ip.To4()) {
		oldIP = old.IP
	}
	p.offers[key] = &pendingOffer{
		IP:     ip.To4(),
		Expiry: time.Now().Add(p.timeout),
	}
	return oldIP
}

// remove deletes the pending offer for mac and returns the offered IP
// so the caller can release it back to the pool. Returns nil if no
// pending offer exists.
func (p *pendingOffers) remove(mac net.HardwareAddr) net.IP {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := mac.String()
	old, ok := p.offers[key]
	if !ok {
		return nil
	}
	delete(p.offers, key)
	return old.IP
}

// ipForMAC returns the offered IP if still valid. If the offer has expired,
// it is removed and the IP is returned as the second value so the caller
// can release it back to the pool.
func (p *pendingOffers) ipForMAC(mac net.HardwareAddr) (active, expired net.IP) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := mac.String()
	o, ok := p.offers[key]
	if !ok {
		return nil, nil
	}
	if time.Now().Before(o.Expiry) {
		return o.IP, nil
	}
	// Expired — reclaim immediately instead of waiting for cleanupLoop.
	delete(p.offers, key)
	return nil, o.IP
}

func (p *pendingOffers) isOfferedTo(mac net.HardwareAddr, ip net.IP) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	o, ok := p.offers[mac.String()]
	return ok && o.IP.Equal(ip) && time.Now().Before(o.Expiry)
}

// expireOld removes expired offers and returns their IPs for pool reclamation.
func (p *pendingOffers) expireOld() []net.IP {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	var expired []net.IP
	for k, o := range p.offers {
		if now.After(o.Expiry) {
			expired = append(expired, o.IP)
			delete(p.offers, k)
		}
	}
	return expired
}
