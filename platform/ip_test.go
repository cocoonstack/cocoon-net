package platform

import (
	"slices"
	"testing"
)

func TestSubnetIPs_Slash24(t *testing.T) {
	t.Parallel()

	// /24 has 256 addresses, 254 hosts, minus the gateway = 253 max.
	got, err := SubnetIPs("10.0.0.0/24", "10.0.0.1", 8)
	if err != nil {
		t.Fatalf("SubnetIPs: %v", err)
	}
	want := []string{
		"10.0.0.2", "10.0.0.3", "10.0.0.4", "10.0.0.5",
		"10.0.0.6", "10.0.0.7", "10.0.0.8", "10.0.0.9",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSubnetIPs_Slash24SkipsBroadcast(t *testing.T) {
	t.Parallel()

	// Ask for more than the subnet can deliver; verify the result
	// excludes both the gateway and the broadcast (10.0.0.255).
	got, err := SubnetIPs("10.0.0.0/24", "10.0.0.1", 300)
	if err != nil {
		t.Fatalf("SubnetIPs: %v", err)
	}
	if slices.Contains(got, "10.0.0.255") {
		t.Errorf("result must not include broadcast 10.0.0.255")
	}
	if slices.Contains(got, "10.0.0.1") {
		t.Errorf("result must not include gateway 10.0.0.1")
	}
	if slices.Contains(got, "10.0.0.0") {
		t.Errorf("result must not include network address 10.0.0.0")
	}
	if len(got) != 253 {
		t.Errorf("expected 253 host IPs in /24, got %d", len(got))
	}
}

func TestSubnetIPs_Slash28(t *testing.T) {
	t.Parallel()

	// /28 has 16 addresses, 14 hosts, minus gateway = 13.
	got, err := SubnetIPs("192.168.10.0/28", "192.168.10.1", 32)
	if err != nil {
		t.Fatalf("SubnetIPs: %v", err)
	}
	want := []string{
		"192.168.10.2", "192.168.10.3", "192.168.10.4", "192.168.10.5",
		"192.168.10.6", "192.168.10.7", "192.168.10.8", "192.168.10.9",
		"192.168.10.10", "192.168.10.11", "192.168.10.12", "192.168.10.13",
		"192.168.10.14",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// Explicit broadcast check.
	if slices.Contains(got, "192.168.10.15") {
		t.Errorf("/28 broadcast 192.168.10.15 must be excluded")
	}
}

func TestSubnetIPs_EmptyGatewayIsError(t *testing.T) {
	t.Parallel()

	if _, err := SubnetIPs("10.0.0.0/24", "", 4); err == nil {
		t.Fatalf("empty gateway must error")
	}
}

func TestSubnetIPs_InvalidGatewayIsError(t *testing.T) {
	t.Parallel()

	if _, err := SubnetIPs("10.0.0.0/24", "not-an-ip", 4); err == nil {
		t.Fatalf("invalid gateway must error")
	}
}

func TestSubnetIPs_InvalidCIDRIsError(t *testing.T) {
	t.Parallel()

	if _, err := SubnetIPs("not-a-cidr", "10.0.0.1", 4); err == nil {
		t.Fatalf("invalid CIDR must error")
	}
}

func TestSubnetIPs_IPv6IsError(t *testing.T) {
	t.Parallel()

	if _, err := SubnetIPs("2001:db8::/64", "2001:db8::1", 4); err == nil {
		t.Fatalf("IPv6 must error")
	}
}

func TestSubnetIPs_GatewayOutsideCIDRIsError(t *testing.T) {
	t.Parallel()

	if _, err := SubnetIPs("10.0.0.0/24", "10.0.1.1", 4); err == nil {
		t.Fatalf("gateway outside cidr must error")
	}
}

func TestSubnetIPs_GatewayEqualsNetworkIsError(t *testing.T) {
	t.Parallel()

	if _, err := SubnetIPs("10.0.0.0/24", "10.0.0.0", 4); err == nil {
		t.Fatalf("gateway == network address must error")
	}
}

func TestSubnetIPs_GatewayEqualsBroadcastIsError(t *testing.T) {
	t.Parallel()

	if _, err := SubnetIPs("10.0.0.0/24", "10.0.0.255", 4); err == nil {
		t.Fatalf("gateway == broadcast must error")
	}
}

func TestSubnetIPs_CountZero(t *testing.T) {
	t.Parallel()

	got, err := SubnetIPs("10.0.0.0/24", "10.0.0.1", 0)
	if err != nil {
		t.Fatalf("SubnetIPs: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("count=0 must return empty, got %v", got)
	}
}

func TestFirstHostIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cidr string
		want string
	}{
		{"10.0.0.0/24", "10.0.0.1"},
		{"192.168.0.0/16", "192.168.0.1"},
		{"172.20.100.0/22", "172.20.100.1"},
	}
	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			t.Parallel()
			got, err := FirstHostIP(tt.cidr)
			if err != nil {
				t.Fatalf("FirstHostIP: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestCIDRContainsCIDR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		outer, inner string
		want         bool
	}{
		{"10.0.0.0/16", "10.0.0.0/24", true},
		{"10.0.0.0/16", "10.0.0.0/16", true},
		{"10.0.0.0/24", "10.0.0.0/16", false},
		{"10.0.0.0/24", "10.1.0.0/24", false},
	}
	for _, tt := range tests {
		t.Run(tt.outer+" contains "+tt.inner, func(t *testing.T) {
			t.Parallel()
			got, err := CIDRContainsCIDR(tt.outer, tt.inner)
			if err != nil {
				t.Fatalf("CIDRContainsCIDR: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIP4ToUint32(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ip   string
		want uint32
	}{
		{"0.0.0.0", 0},
		{"127.0.0.1", 0x7F000001},
		{"255.255.255.255", 0xFFFFFFFF},
		{"10.0.0.1", 0x0A000001},
		{"not-an-ip", 0},
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			t.Parallel()
			if got := IP4ToUint32(tt.ip); got != tt.want {
				t.Errorf("got %#x, want %#x", got, tt.want)
			}
		})
	}
}
