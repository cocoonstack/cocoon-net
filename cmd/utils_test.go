package cmd

import (
	"slices"
	"testing"
)

func TestSplitTrim(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty string returns nil", "", nil},
		{"whitespace only returns nil", "   ", nil},
		{"single value", "a", []string{"a"}},
		{"single value with whitespace", "  a  ", []string{"a"}},
		{"two values", "a,b", []string{"a", "b"}},
		{"two values with whitespace", " a , b ", []string{"a", "b"}},
		{"trailing comma is dropped", "a,", []string{"a"}},
		{"leading comma is dropped", ",a", []string{"a"}},
		{"empties between values are dropped", "a,,b", []string{"a", "b"}},
		{"all empties returns nil", ",,", nil},
		{"dns example", "8.8.8.8,1.1.1.1", []string{"8.8.8.8", "1.1.1.1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := splitTrim(tt.input, ",")
			if !slices.Equal(got, tt.want) {
				t.Errorf("splitTrim(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveSubnet(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"host bits masked", "172.20.100.5/24", "172.20.100.0/24", false},
		{"already canonical", "10.0.0.0/8", "10.0.0.0/8", false},
		{"invalid", "not-a-cidr", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flagSubnet = tt.in
			err := resolveSubnet()
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveSubnet(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if !tt.wantErr && flagSubnet != tt.want {
				t.Errorf("flagSubnet = %q, want %q", flagSubnet, tt.want)
			}
		})
	}
}

func TestResolveLeaseFile(t *testing.T) {
	tests := []struct {
		name      string
		stateDir  string
		leaseFile string
		want      string
	}{
		{"default state dir", defaultStateDir, "", "/var/lib/cocoon/net/leases.json"},
		{"custom state dir", "/srv/cocoon/net", "", "/srv/cocoon/net/leases.json"},
		{"explicit lease file wins", "/srv/cocoon/net", "/run/cocoon/leases.json", "/run/cocoon/leases.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flagStateDir, flagLeaseFile = tt.stateDir, tt.leaseFile
			if got := resolveLeaseFile(); got != tt.want {
				t.Errorf("resolveLeaseFile() = %q, want %q", got, tt.want)
			}
		})
	}
}
