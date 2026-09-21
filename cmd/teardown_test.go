package cmd

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestTeardownLeaseFileFlag(t *testing.T) {
	cmd := newTeardownCmd()
	flag := cmd.Flags().Lookup("lease-file")
	if flag == nil {
		t.Fatal("teardown has no lease-file flag")
	}
	if flag.DefValue != defaultLeaseFile {
		t.Fatalf("lease-file default = %q, want %q", flag.DefValue, defaultLeaseFile)
	}
}

func TestTeardownForceFlag(t *testing.T) {
	if newTeardownCmd().Flags().Lookup("force") == nil {
		t.Fatal("teardown has no force flag")
	}
}

func TestCheckExistingPIDRefusesALiveDaemon(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cocoon-net.pid")
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkExistingPID(path); err == nil {
		t.Fatal("a pidfile naming a live process must be refused")
	}
	if err := os.WriteFile(path, []byte("999999"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkExistingPID(path); err != nil {
		t.Fatalf("a stale pidfile must not block: %v", err)
	}
	if err := checkExistingPID(filepath.Join(t.TempDir(), "absent")); err != nil {
		t.Fatalf("a missing pidfile must not block: %v", err)
	}
}
