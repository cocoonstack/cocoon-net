package cmd

import "testing"

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
