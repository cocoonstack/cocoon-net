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
