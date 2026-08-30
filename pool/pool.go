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

	"github.com/cocoonstack/cocoon-net/utils"
)

const (
	poolFileName = "pool.json"

	filePerm = 0o644
	dirPerm  = 0o750
)

// State represents the pool state persisted to disk.
type State struct {
	Platform   string `json:"platform"`
	NodeName   string `json:"nodeName"`
	Subnet     string `json:"subnet"`
	Gateway    string `json:"gateway"`
	PrimaryNIC string `json:"primaryNIC,omitempty"`

	StateDir string `json:"-"`

	SecondaryNICs []string `json:"secondaryNICs,omitempty"`
	IPs           []string `json:"ips"`
	ENIIDs        []string `json:"eniIDs,omitempty"`

	// AliasRangeName is the GCE secondary range (GKE only); empty makes teardown use the default.
	AliasRangeName string `json:"aliasRangeName,omitempty"`

	// DNSServers is handed out in DHCP replies; empty on old state, the daemon then uses built-in defaults.
	DNSServers []string `json:"dnsServers,omitempty"`

	// DropInternalAccess and DropCIDRs become FORWARD DROP rules at node setup.
	DropInternalAccess bool     `json:"dropInternalAccess,omitempty"`
	DropCIDRs          []string `json:"dropCIDRs,omitempty"`

	UpdatedAt time.Time `json:"updatedAt"`
}

// Save writes pool.json atomically.
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
	if err := utils.WriteFileAtomic(path, data, filePerm); err != nil {
		return fmt.Errorf("save pool state: %w", err)
	}
	logger.Infof(ctx, "pool state saved (%d IPs) to %s", len(s.IPs), path)
	return nil
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

// Load reads stateDir/pool.json; a leftover .tmp is ignored since Save commits by rename.
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
