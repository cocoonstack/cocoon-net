package cmd

import (
	"context"
	"slices"
	"testing"

	"github.com/cocoonstack/cocoon-net/platform"
)

func TestAdoptPoolVolcengineKeepsSubnetENIs(t *testing.T) {
	plat := stubPlatform{name: platform.PlatformVolcengine, status: &platform.PoolStatus{ENIs: []platform.ENIStatus{
		{ID: "eni-a", MAC: "00:16:3e:00:00:0a", IPs: []string{"10.0.1.20", "10.0.1.10", "10.0.1.1", "10.0.1.255"}},
		{ID: "eni-b", MAC: "00:16:3e:00:00:0b", IPs: []string{"10.0.9.5"}},
		{ID: "eni-c", MAC: "00:16:3e:00:00:0c", IPs: []string{"10.0.1.30", "10.0.1.20"}},
	}}}
	ips, enis, err := adoptPool(t.Context(), plat, "10.0.1.0/24", "10.0.1.1", 0)
	if err != nil {
		t.Fatalf("adoptPool: %v", err)
	}
	if want := []string{"10.0.1.10", "10.0.1.20", "10.0.1.30"}; !slices.Equal(ips, want) {
		t.Fatalf("ips = %v, want %v", ips, want)
	}
	if len(enis) != 2 || enis[0].ID != "eni-a" || enis[1].ID != "eni-c" {
		t.Fatalf("contributing ENIs = %+v, want eni-a and eni-c", enis)
	}
}

func TestAdoptPoolVolcengineWithoutSubnetIPsIsError(t *testing.T) {
	plat := stubPlatform{name: platform.PlatformVolcengine, status: &platform.PoolStatus{ENIs: []platform.ENIStatus{
		{ID: "eni-b", MAC: "00:16:3e:00:00:0b", IPs: []string{"10.0.9.5"}},
	}}}
	if _, _, err := adoptPool(t.Context(), plat, "10.0.1.0/24", "10.0.1.1", 0); err == nil {
		t.Fatal("adoptPool accepted a node without pool IPs")
	}
}

func TestAdoptPoolGKEUsesSubnet(t *testing.T) {
	ips, enis, err := adoptPool(t.Context(), stubPlatform{name: platform.PlatformGKE}, "10.0.1.0/28", "10.0.1.1", 3)
	if err != nil {
		t.Fatalf("adoptPool: %v", err)
	}
	if want := []string{"10.0.1.2", "10.0.1.3", "10.0.1.4"}; !slices.Equal(ips, want) || enis != nil {
		t.Fatalf("ips = %v enis = %v, want %v and no ENIs", ips, enis, want)
	}
}

func TestRequirePoolLinksMatchesByMAC(t *testing.T) {
	enis := []platform.ENIStatus{{ID: "eni-a", MAC: "00:16:3E:00:00:0A"}}
	if err := requirePoolLinks(enis, map[string]string{"eth1": "00:16:3e:00:00:0a", "eth2": "00:16:3e:00:00:0b"}); err != nil {
		t.Fatalf("matching link rejected: %v", err)
	}
	if err := requirePoolLinks(enis, map[string]string{"eth2": "00:16:3e:00:00:0b"}); err == nil {
		t.Fatal("ENI without a link accepted because another link is present")
	}
	if err := requirePoolLinks(nil, nil); err != nil {
		t.Fatalf("no pool ENIs rejected: %v", err)
	}
}

type stubPlatform struct {
	platform.CloudPlatform
	name   string
	status *platform.PoolStatus
}

func (s stubPlatform) Name() string { return s.name }

func (s stubPlatform) Status(context.Context) (*platform.PoolStatus, error) { return s.status, nil }
