package volcengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cocoonstack/cocoon-net/platform"
)

func TestTeardownDeletesDetachedPersistedENI(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$VE_TEST_LOG"
case "$*" in
  *DescribeNetworkInterfaces*)
    printf '%s\n' '{"Result":{"NetworkInterfaceSets":[{"NetworkInterfaceId":"eni-detached","Type":"secondary","DeviceId":""}]}}'
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "ve"), []byte(script), 0o755); err != nil {
		t.Fatalf("write ve stub: %v", err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("VE_TEST_LOG", logPath)

	v := &Volcengine{}
	if err := v.Teardown(t.Context(), &platform.TeardownConfig{ENIIDs: []string{"eni-detached"}}); err != nil {
		t.Fatalf("teardown: %v", err)
	}

	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read calls: %v", err)
	}
	want := strings.Join([]string{
		"vpc DescribeNetworkInterfaces --NetworkInterfaceIds.1 eni-detached --PageSize 100",
		"vpc DeleteNetworkInterface --NetworkInterfaceId eni-detached",
	}, "\n")
	if got := strings.TrimSpace(string(calls)); got != want {
		t.Fatalf("got calls %q, want %q", got, want)
	}
}
