package volcengine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	eniList1Primary7Secondary = `{
  "Result": {
    "NetworkInterfaceSets": [
      {"NetworkInterfaceId": "eni-primary", "Type": "primary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": true, "PrivateIpAddress": "10.0.0.1"}]}},
      {"NetworkInterfaceId": "eni-1", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.2"}]}},
      {"NetworkInterfaceId": "eni-2", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.3"}]}},
      {"NetworkInterfaceId": "eni-3", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.4"}]}},
      {"NetworkInterfaceId": "eni-4", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.5"}]}},
      {"NetworkInterfaceId": "eni-5", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.6"}]}},
      {"NetworkInterfaceId": "eni-6", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.7"}]}},
      {"NetworkInterfaceId": "eni-7", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.8"}]}}
    ]
  }
}`

	eniList1Primary3Secondary = `{
  "Result": {
    "NetworkInterfaceSets": [
      {"NetworkInterfaceId": "eni-primary", "Type": "primary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": true, "PrivateIpAddress": "10.0.0.1"}]}},
      {"NetworkInterfaceId": "eni-1", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.2"}]}},
      {"NetworkInterfaceId": "eni-2", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.3"}]}},
      {"NetworkInterfaceId": "eni-3", "Type": "secondary", "PrivateIpSets": {"PrivateIpSet": [{"Primary": false, "PrivateIpAddress": "10.0.0.4"}]}}
    ]
  }
}`
)

func TestReusableENIs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fixture string
		count   int
		want    int
	}{
		{"exact match runs zero shortfall", eniList1Primary7Secondary, 7, 7},
		{"fewer than count returns all", eniList1Primary3Secondary, 7, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			enis := unmarshalENIList(t, tt.fixture)
			if got := len(selectReusableENIs(enis, tt.count)); got != tt.want {
				t.Errorf("got %d reusable ENIs, want %d", got, tt.want)
			}
		})
	}
}

func TestEnsureENIsDeletesTheOrphanWhenTheAttachIsCanceled(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	attached := filepath.Join(dir, "attached")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$VE_TEST_LOG"
case "$*" in
  *CreateNetworkInterface*) printf '%s\n' '{"Result":{"NetworkInterfaceId":"eni-orphan"}}' ;;
  *AttachNetworkInterface*) : > "$VE_TEST_ATTACHED"; sleep 5 ;;
  *DescribeNetworkInterfaces*) printf '%s\n' '{"Result":{"NetworkInterfaceSets":[]}}' ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "ve"), []byte(script), 0o755); err != nil {
		t.Fatalf("write ve stub: %v", err)
	}
	t.Setenv("PATH", dir+":/bin:/usr/bin")
	t.Setenv("VE_TEST_LOG", logPath)
	t.Setenv("VE_TEST_ATTACHED", attached)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := ensureENIs(ctx, "subnet-1", "sg-1", "i-1", "cocoon-pool", 1)
		done <- err
	}()
	waitForFile(t, attached)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the cancellation", err)
	}
	calls, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read calls: %v", readErr)
	}
	if !strings.Contains(string(calls), "DeleteNetworkInterface --NetworkInterfaceId eni-orphan") {
		t.Fatalf("orphan ENI not deleted after the canceled attach, calls:\n%s", calls)
	}
}

func unmarshalENIList(t *testing.T, fixture string) []networkInterface {
	t.Helper()

	var resp struct {
		Result struct {
			NetworkInterfaceSets []networkInterface `json:"NetworkInterfaceSets"`
		} `json:"Result"`
	}
	if err := json.Unmarshal([]byte(fixture), &resp); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return resp.Result.NetworkInterfaceSets
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never appeared", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
