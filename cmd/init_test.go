package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cocoonstack/cocoon-net/pool"
)

func TestInitRetainsFailedENICleanupAcrossSuccessfulProvisioning(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$VE_TEST_LOG"
case "$2" in
  DescribeSecurityGroups) printf '%s\n' '{"Result":{"SecurityGroups":[{"SecurityGroupId":"sg-1"}]}}' ;;
  DescribeSubnets) printf '%s\n' '{"Result":{"Subnets":[{"SubnetId":"subnet-1","CidrBlock":"10.0.0.0/24"}]}}' ;;
  DescribeNetworkInterfaces)
    printf '%s' '{"Result":{"NetworkInterfaceSets":['
    if [ -f "$VE_TEST_RECOVERED" ]; then
      sep=''
      for i in 1 2 3 4 5 6 7; do
        printf '%s{"NetworkInterfaceId":"eni-%s","Type":"secondary","DeviceId":"i-1"}' "$sep" "$i"
        sep=','
      done
    fi
    printf '%s\n' ']}}'
    ;;
  CreateNetworkInterface) printf '%s\n' '{"Result":{"NetworkInterfaceId":"eni-orphan"}}' ;;
  AttachNetworkInterface) echo 'attach unavailable' >&2; exit 1 ;;
  DeleteNetworkInterface) : > "$VE_TEST_RECOVERED"; echo 'delete unavailable' >&2; exit 1 ;;
  AssignPrivateIpAddresses) printf '{"Result":{"PrivateIpSet":["10.0.0.%s"]}}\n' "${4#eni-}" ;;
  *) echo 'unexpected API call' >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "ve"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("VE_TEST_LOG", logPath)
	t.Setenv("VE_TEST_RECOVERED", filepath.Join(dir, "recovered"))
	t.Setenv("VOLCENGINE_ACCESS_KEY_ID", "test")
	transport := http.DefaultTransport
	http.DefaultTransport = initMetadataTransport{}
	t.Cleanup(func() { http.DefaultTransport = transport })

	stateDir := filepath.Join(dir, "state")
	prior := &pool.State{StateDir: stateDir, ENIIDs: []string{"eni-deleted"}, IPs: []string{"10.0.0.20"}}
	if err := prior.Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	args := []string{"--platform", "volcengine", "--node-name", "test", "--subnet", "10.0.0.0/24", "--state-dir", stateDir, "--primary-nic", "cocoon-net-test-missing"}
	first := newInitCmd()
	first.SetArgs(args)
	if err := first.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), "attach unavailable") || !strings.Contains(err.Error(), "delete orphan ENI eni-orphan") {
		t.Fatalf("init error = %v, want attach and cleanup failures", err)
	}
	failed, err := pool.Load(t.Context(), stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(failed.ENIIDs, []string{"eni-deleted", "eni-orphan"}) || !slices.Equal(failed.IPs, prior.IPs) {
		t.Fatalf("failed init lost ownership or changed the existing pool: %+v", failed)
	}
	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(calls), "AssignPrivateIpAddresses") {
		t.Fatalf("failed ENI was used for IP allocation: %s", calls)
	}

	retry := newInitCmd()
	retry.SetArgs(args)
	if err := retry.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), "node setup: primary NIC:") {
		t.Fatalf("retry error = %v, want the deliberately absent NIC after provisioning", err)
	}
	provisioned, err := pool.Load(t.Context(), stateDir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"eni-deleted", "eni-orphan", "eni-1", "eni-2", "eni-3", "eni-4", "eni-5", "eni-6", "eni-7"}
	if !slices.Equal(provisioned.ENIIDs, want) || len(provisioned.IPs) != 7 {
		t.Fatalf("successful provisioning lost prior cleanup ownership: %+v", provisioned)
	}
}

type initMetadataTransport struct{}

func (initMetadataTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body string
	switch req.URL.String() {
	case "http://100.96.0.96/latest/meta-data/vpc-id":
		body = "vpc-1"
	case "http://100.96.0.96/latest/meta-data/instance-id":
		body = "i-1"
	default:
		return nil, fmt.Errorf("unexpected metadata request: %s", req.URL)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
}
