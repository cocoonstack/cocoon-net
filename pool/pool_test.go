package pool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestState_SaveLoadRoundtrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ctx := t.Context()

	src := &State{
		Platform:       "gke",
		NodeName:       "node-a",
		Subnet:         "10.0.0.0/24",
		Gateway:        "10.0.0.1",
		PrimaryNIC:     "ens4",
		StateDir:       dir,
		SecondaryNICs:  []string{"eth1", "eth2"},
		IPs:            []string{"10.0.0.10", "10.0.0.11"},
		ENIIDs:         []string{"eni-1"},
		SubnetID:       "subnet-x",
		AliasRangeName: "cocoon-pods",
		DNSServers:     []string{"8.8.8.8"},
	}
	if err := src.Save(ctx); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, poolFileName)); err != nil {
		t.Fatalf("pool.json missing: %v", err)
	}

	got, err := Load(ctx, dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Platform != src.Platform || got.NodeName != src.NodeName || got.Subnet != src.Subnet {
		t.Errorf("identification roundtrip mismatch: got %+v", got)
	}
	if len(got.IPs) != len(src.IPs) || got.IPs[0] != src.IPs[0] || got.IPs[1] != src.IPs[1] {
		t.Errorf("IPs roundtrip mismatch: got %v want %v", got.IPs, src.IPs)
	}
	if got.AliasRangeName != src.AliasRangeName {
		t.Errorf("AliasRangeName roundtrip mismatch: %q vs %q", got.AliasRangeName, src.AliasRangeName)
	}
	if got.StateDir != dir {
		t.Errorf("StateDir not restored: %q vs %q", got.StateDir, dir)
	}
	if got.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt zero after roundtrip")
	}
}

// A bogus pool.json.tmp must not interfere with Load or a subsequent Save.
func TestState_SaveAtomicTmpIgnored(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ctx := t.Context()

	good := &State{
		Platform: "gke",
		NodeName: "node-a",
		Subnet:   "10.0.0.0/24",
		Gateway:  "10.0.0.1",
		StateDir: dir,
		IPs:      []string{"10.0.0.10"},
	}
	if err := good.Save(ctx); err != nil {
		t.Fatalf("save: %v", err)
	}

	tmp := filepath.Join(dir, poolFileName+".tmp")
	if err := os.WriteFile(tmp, []byte("{not json"), filePerm); err != nil {
		t.Fatalf("write fake tmp: %v", err)
	}

	got, err := Load(ctx, dir)
	if err != nil {
		t.Fatalf("load with .tmp present must succeed: %v", err)
	}
	if got.NodeName != "node-a" {
		t.Errorf("Load picked up wrong file: NodeName=%q", got.NodeName)
	}
	if _, err := os.Stat(tmp); err != nil {
		t.Errorf(".tmp should still exist after Load: %v", err)
	}

	if err := got.Save(ctx); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	reloaded, err := Load(ctx, dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.NodeName != "node-a" {
		t.Errorf("reload after Save corrupted state: %q", reloaded.NodeName)
	}
}

func TestState_LoadMissingFile(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	if _, err := Load(ctx, t.TempDir()); err == nil {
		t.Fatalf("Load on empty dir must return error")
	}
}

func TestState_Delete(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ctx := t.Context()
	s := &State{
		Platform: "gke",
		NodeName: "n",
		Subnet:   "10.0.0.0/24",
		StateDir: dir,
		IPs:      []string{"10.0.0.10"},
	}
	if err := s.Save(ctx); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := s.Delete(ctx); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.Delete(ctx); err != nil {
		t.Errorf("second Delete must be idempotent: %v", err)
	}
}
