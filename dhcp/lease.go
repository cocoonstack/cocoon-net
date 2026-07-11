package dhcp

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const leaseFilePerm = 0o644

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

// evictedLease describes a lease entry displaced by add(). Same-MAC
// rebind leaves the old IP's route + pool slot orphaned; other-MAC
// conflict is reported for logging.
type evictedLease struct {
	MAC string
	IP  net.IP
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

// add commits a lease, returning any prior entries it displaced.
func (s *leaseStore) add(mac net.HardwareAddr, ip net.IP, duration time.Duration) []evictedLease {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	key := mac.String()
	newIP := ip.To4()
	var evicted []evictedLease

	// Same MAC, different IP — surface the old IP so the caller can clean it up.
	if prev, ok := s.leases[key]; ok && !prev.IP.Equal(newIP) {
		evicted = append(evicted, evictedLease{MAC: key, IP: prev.IP})
	}

	// Other MAC, same IP — drop the conflicting active lease.
	for k, l := range s.leases {
		if l.IP.Equal(newIP) && k != key && now.Before(l.Expiry) {
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
	// Atomic write via a UNIQUE temp file per call, then rename. save() holds
	// only RLock, so concurrent savers (request/release/cleanup goroutines) must
	// not share a fixed temp path or their O_TRUNC writes interleave and rename
	// publishes a corrupt leases.json.
	tmp, err := os.CreateTemp(filepath.Dir(s.filePath), "leases-*.json")
	if err != nil {
		return fmt.Errorf("create leases tmp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write leases tmp: %w", err)
	}
	if err := tmp.Chmod(leaseFilePerm); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod leases tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close leases tmp: %w", err)
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename leases: %w", err)
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
