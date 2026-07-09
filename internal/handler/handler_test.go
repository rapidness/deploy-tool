package handler

import (
	"testing"
)

func TestParseServiceNames(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"A", []string{"A"}},
		{"A,B,C", []string{"A", "B", "C"}},
		{" A , B , C ", []string{"A", "B", "C"}},
	}
	for _, c := range cases {
		got := ParseServiceNames(c.in)
		if len(got) != len(c.want) {
			t.Errorf("ParseServiceNames(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("ParseServiceNames(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}
