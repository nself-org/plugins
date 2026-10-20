package sbom

import (
	"slices"
	"testing"
)

func TestStripCheckDropsOnlyTheTrailingEntry(t *testing.T) {
	cases := []struct {
		name     string
		in, want []string
	}{
		{"trailing", []string{"sh", "-c", "make dist", CheckID}, []string{"sh", "-c", "make dist"}},
		{"padded", []string{"make", " " + CheckID + "\n"}, []string{"make"}},
		{"only entry", []string{CheckID}, []string{}},
		{"not last", []string{CheckID, "make"}, []string{CheckID, "make"}},
		{"mentioned", []string{"echo", CheckID + " done"}, []string{"echo", CheckID + " done"}},
		{"absent", []string{"make"}, []string{"make"}},
		{"nil", nil, nil},
	}
	for _, c := range cases {
		orig := slices.Clone(c.in)
		got := StripCheck(c.in)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: StripCheck(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
		if !slices.Equal(c.in, orig) {
			t.Errorf("%s: the input was modified: %q", c.name, c.in)
		}
	}
}
