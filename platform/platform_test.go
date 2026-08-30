package platform

import (
	"slices"
	"testing"
)

func TestSecondaryNICNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		n    int
		want []string
	}{
		{"zero", 0, []string{}},
		{"three", 3, []string{"eth1", "eth2", "eth3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := SecondaryNICNames(tt.n); !slices.Equal(got, tt.want) {
				t.Errorf("SecondaryNICNames(%d) = %v, want %v", tt.n, got, tt.want)
			}
		})
	}
}
