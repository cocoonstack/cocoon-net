//go:build linux

package node

import (
	"strings"
	"testing"
)

func TestRuleDest(t *testing.T) {
	tests := []struct {
		name string
		rule string
		want string
		ok   bool
	}{
		{"drop rule", "-A FORWARD -i cni0 -d 10.88.0.0/24 -m comment --comment cocoon-net-drop -j DROP", "10.88.0.0/24", true},
		{"single host", "-A FORWARD -i cni0 -d 10.0.0.5/32 -j DROP", "10.0.0.5/32", true},
		{"no destination", "-A FORWARD -i cni0 -o cni0 -j ACCEPT", "", false},
		{"trailing -d", "-A FORWARD -i cni0 -d", "", false},
		{"empty", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ruleDest(strings.Fields(tt.rule))
			if got != tt.want || ok != tt.ok {
				t.Errorf("ruleDest(%q) = (%q, %v), want (%q, %v)", tt.rule, got, ok, tt.want, tt.ok)
			}
		})
	}
}
