package strutil

import "testing"

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{"first is non-empty", []string{"a", "b"}, "a"},
		{"first is empty", []string{"", "b"}, "b"},
		{"first is whitespace", []string{"  ", "b"}, "b"},
		{"all empty", []string{"", "", ""}, ""},
		{"no values", nil, ""},
		{"single value", []string{"x"}, "x"},
		{"preserves original (no trim on return)", []string{"  hello  "}, "  hello  "},
		{"tabs are whitespace", []string{"\t", "ok"}, "ok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FirstNonEmpty(tt.values...)
			if got != tt.want {
				t.Errorf("FirstNonEmpty(%v) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}
