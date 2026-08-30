package dhcp

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/cocoonstack/cocoon-net/utils"
)

const leaseFilePerm = 0o644

type lease struct {
	MAC    net.HardwareAddr
	IP     net.IP
	Expiry time.Time
}

type leaseEntry struct {
	MAC    string `json:"mac"`
	IP     string `json:"ip"`
	Expiry string `json:"expiry"`
}

// evictedLease is a lease displaced by add(): a same-MAC rebind orphans the old IP's route and pool slot, an other-MAC conflict is log-only.
type evictedLease struct {
	MAC string
	IP  net.IP
}

type leaseStore struct {
	mu       sync.RWMutex
	leases   map[string]*lease // keyed by MAC string
	filePath string
}

func newLeaseStore(filePath string) *leaseStore {
	return &leaseStore{
		leases:   make(map[string]*lease),
		filePath: filePath,
	}
}

func (s *leaseStore) add(mac net.HardwareAddr, ip net.IP, duration time.Duration) []evictedLease {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	key := mac.String()
	newIP := ip.To4()
	var evicted []evictedLease

	if prev, ok := s.leases[key]; ok && !prev.IP.Equal(newIP) {
		evicted = append(evicted, evictedLease{MAC: key, IP: prev.IP})
	}

	for k, l := range s.leases {
		if l.IP.Equal(newIP) && k != key {
			delete(s.leases, k)
			evicted = append(evicted, evictedLease{MAC: k, IP: l.IP})
		}
	}

	s.leases[key] = &lease{
		MAC:    mac,
		IP:     newIP,
		Expiry: now.Add(duration),
	}
	return evicted
}

// take also returns an expired entry so an explicit release reclaims its slot before the sweep.
func (s *leaseStore) take(mac net.HardwareAddr) *lease {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := mac.String()
	l, ok := s.leases[key]
	if !ok {
		return nil
	}
	delete(s.leases, key)
	cp := *l
	return &cp
}

func (s *leaseStore) restore(l *lease) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leases[l.MAC.String()] = l
}

func (s *leaseStore) restoreAll(leases []lease) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range leases {
		s.leases[l.MAC.String()] = &l
	}
}

func (s *leaseStore) ipForMAC(mac net.HardwareAddr) net.IP {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if l, ok := s.leases[mac.String()]; ok && time.Now().Before(l.Expiry) {
		return l.IP
	}
	return nil
}

func (s *leaseStore) isLeasedTo(mac net.HardwareAddr, ip net.IP) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.leases[mac.String()]
	return ok && l.IP.Equal(ip) && time.Now().Before(l.Expiry)
}

func (s *leaseStore) isLeasedToOther(mac net.HardwareAddr, ip net.IP) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	for k, l := range s.leases {
		if l.IP.Equal(ip) && now.Before(l.Expiry) && k != mac.String() {
			return true
		}
	}
	return false
}

func (s *leaseStore) activeLeases() []lease {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	var active []lease
	for _, l := range s.leases {
		if now.Before(l.Expiry) {
			active = append(active, *l)
		}
	}
	return active
}

func (s *leaseStore) activeCount() int {
	return len(s.activeLeases())
}

func (s *leaseStore) expireOld() []lease {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var expired []lease
	for k, l := range s.leases {
		if now.After(l.Expiry) {
			expired = append(expired, *l)
			delete(s.leases, k)
		}
	}
	return expired
}

func (s *leaseStore) save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var entries []leaseEntry
	for _, l := range s.leases {
		entries = append(entries, leaseEntry{
			MAC:    l.MAC.String(),
			IP:     l.IP.String(),
			Expiry: l.Expiry.Format(time.RFC3339),
		})
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal leases: %w", err)
	}
	// every caller holds lifecycleMu, so the fixed tmp path has a single writer
	if err := utils.WriteFileAtomic(s.filePath, data, leaseFilePerm); err != nil {
		return fmt.Errorf("save leases: %w", err)
	}
	return nil
}

func (s *leaseStore) load() error {
	data, err := os.ReadFile(s.filePath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("read leases from %s: %w", s.filePath, err)
	}
	var entries []leaseEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parse leases: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, e := range entries {
		mac, parseErr := net.ParseMAC(e.MAC)
		if parseErr != nil {
			continue
		}
		ip := net.ParseIP(e.IP).To4()
		if ip == nil {
			continue
		}
		expiry, parseErr := time.Parse(time.RFC3339, e.Expiry)
		if parseErr != nil || now.After(expiry) {
			continue
		}
		s.leases[mac.String()] = &lease{MAC: mac, IP: ip, Expiry: expiry}
	}
	return nil
}
