package dhcp

import (
	"encoding/json"
	"net"
	"os"
	"sync"
	"time"
)

// lease represents a single DHCP lease.
type lease struct {
	MAC    net.HardwareAddr
	IP     net.IP
	Expiry time.Time
}

// leaseEntry is the JSON-serializable form of a lease.
type leaseEntry struct {
	MAC    string `json:"mac"`
	IP     string `json:"ip"`
	Expiry string `json:"expiry"`
}

// leaseStore manages active leases with persistence to a JSON file.
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

func (s *leaseStore) add(mac net.HardwareAddr, ip net.IP, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leases[mac.String()] = &lease{
		MAC:    mac,
		IP:     ip.To4(),
		Expiry: time.Now().Add(duration),
	}
}

func (s *leaseStore) remove(mac net.HardwareAddr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.leases, mac.String())
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

// isLeasedToOther returns true if ip is actively leased to a different MAC.
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

// expireOld removes expired leases and returns them.
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
		return err
	}
	// Atomic write: temp file + rename to prevent corruption on crash.
	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil { //nolint:gosec
		return err
	}
	return os.Rename(tmp, s.filePath)
}

func (s *leaseStore) load() error {
	data, err := os.ReadFile(s.filePath) //nolint:gosec
	if err != nil {
		return err
	}
	var entries []leaseEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
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
