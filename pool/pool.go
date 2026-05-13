// Package pool persists the cocoon-net IP allocation pool to disk.
package pool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/projecteru2/core/log"
)

const (
	poolFileName = "pool.json"

	filePerm = 0o644
	dirPerm  = 0o750
)

// State represents the pool state persisted to disk.
type State struct {
	// identification / config
	Platform   string `json:"platform"`
	NodeName   string `json:"nodeName"`
	Subnet     string `json:"subnet"`
	Gateway    string `json:"gateway"`
	PrimaryNIC string `json:"primaryNIC,omitempty"`

	// runtime (not persisted)
	StateDir string `json:"-"`

	// resources
	SecondaryNICs []string `json:"secondaryNICs,omitempty"`
	IPs           []string `json:"ips"`
	ENIIDs        []string `json:"eniIDs,omitempty"`
	SubnetID      string   `json:"subnetID,omitempty"`

	// AliasRangeName is the GCE secondary range name (GKE only). Empty
	// for other platforms and for adopted nodes — teardown then falls
	// back to the platform default.
	AliasRangeName string `json:"aliasRangeName,omitempty"`

	// DNSServers handed out in DHCP replies. Empty on state written
	// before this field existed; daemon falls back to built-in defaults.
	DNSServers []string `json:"dnsServers,omitempty"`

	// timestamps
	UpdatedAt time.Time `json:"updatedAt"`
}

// Save atomically writes the pool state via tmp+rename, so a crash
// mid-write leaves a stale .tmp but never a partial pool.json.
func (s *State) Save(ctx context.Context) error {
	logger := log.WithFunc("pool.Save")

	if err := os.MkdirAll(s.StateDir, dirPerm); err != nil {
		return fmt.Errorf("create state dir %s: %w", s.StateDir, err)
	}

	s.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal pool state: %w", err)
	}

	path := filepath.Join(s.StateDir, poolFileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, filePerm); err != nil { //nolint:gosec // not sensitive
		return fmt.Errorf("write pool state tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename pool state: %w", err)
	}
	logger.Infof(ctx, "pool state saved (%d IPs) to %s", len(s.IPs), path)
	return nil
}

// Load reads the pool state from stateDir/pool.json. A leftover
// pool.json.tmp is ignored — Save commits via rename, so .tmp is by
// definition incomplete.
func Load(ctx context.Context, stateDir string) (*State, error) {
	logger := log.WithFunc("pool.Load")

	path := filepath.Join(stateDir, poolFileName)
	data, err := os.ReadFile(path) //nolint:gosec // known path
	if err != nil {
		return nil, fmt.Errorf("read pool state from %s: %w", path, err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse pool state: %w", err)
	}
	s.StateDir = stateDir
	logger.Infof(ctx, "pool state loaded (%d IPs, platform=%s)", len(s.IPs), s.Platform)
	return &s, nil
}

// Delete removes the pool state file.
func (s *State) Delete(ctx context.Context) error {
	logger := log.WithFunc("pool.Delete")
	path := filepath.Join(s.StateDir, poolFileName)
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove pool state %s: %w", path, err)
	}
	logger.Infof(ctx, "pool state deleted: %s", path)
	return nil
}
