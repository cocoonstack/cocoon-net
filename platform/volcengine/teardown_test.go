package volcengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestTeardownKeepsTheDetachErrorsACutWaitWouldDrop(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$VE_TEST_LOG"
case "$*" in
  *DescribeNetworkInterfaces*)
    printf '%s\n' '{"Result":{"NetworkInterfaceSets":[{"NetworkInterfaceId":"eni-1","Type":"secondary","DeviceId":"i-1"},{"NetworkInterfaceId":"eni-2","Type":"secondary","DeviceId":"i-1"}]}}'
    ;;
  *DetachNetworkInterface*eni-1*) echo "detach refused" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "ve"), []byte(script), 0o755); err != nil {
		t.Fatalf("write ve stub: %v", err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("VE_TEST_LOG", logPath)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	err := (&Volcengine{}).Teardown(ctx, &platform.TeardownConfig{ENIIDs: []string{"eni-1", "eni-2"}})
	if err == nil || !strings.Contains(err.Error(), "detach ENI eni-1") || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the eni-1 detach failure joined with the cut wait", err)
	}
	if strings.Contains(err.Error(), "detach ENI eni-2") {
		t.Fatalf("the deadline hit the eni-2 detach instead of the propagation wait: %v", err)
	}
	calls, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read calls: %v", readErr)
	}
	if !strings.Contains(string(calls), "DetachNetworkInterface --NetworkInterfaceId eni-2") {
		t.Fatalf("the eni-2 detach never ran, calls:\n%s", calls)
	}
}
