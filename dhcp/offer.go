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

func (p *pendingOffers) add(mac net.HardwareAddr, ip net.IP) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.offers[mac.String()] = &pendingOffer{
		IP:     ip.To4(),
		Expiry: time.Now().Add(p.timeout),
	}
}

func (p *pendingOffers) remove(mac net.HardwareAddr) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.offers, mac.String())
}

func (p *pendingOffers) ipForMAC(mac net.HardwareAddr) net.IP {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if o, ok := p.offers[mac.String()]; ok && time.Now().Before(o.Expiry) {
		return o.IP
	}
	return nil
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
