package gke

import "testing"

func TestResolveAliasRangeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rangeName string
		want      string
	}{
		{"empty falls back to default", "", aliasRangeName},
		{"explicit range name is kept", "custom-range", "custom-range"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveAliasRangeName(tt.rangeName); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}
