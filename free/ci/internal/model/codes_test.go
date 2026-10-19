package model

import (
	"strings"
	"testing"
)

func codeRow(id string) registration {
	return registration{Code{ID: id, Class: "infra", Summary: "s", Fix: "f"}, "test"}
}

// TestAutoCodeBlock: the AUTO block E790-E829 is accepted; the closed gap
// E720-E789 and everything outside both blocks is refused.
func TestAutoCodeBlock(t *testing.T) {
	for _, id := range []string{"E600", "E719", "E790", "E809", "E829"} {
		if got := problems([]registration{codeRow(id)}); len(got) != 0 {
			t.Errorf("%s must be accepted: %v", id, got)
		}
	}
	for _, id := range []string{"E599", "E720", "E789", "E830", "E8290", "X800", "E80"} {
		got := strings.Join(problems([]registration{codeRow(id)}), "\n")
		if !strings.Contains(got, "outside E600-E719 and E790-E829") {
			t.Errorf("%s must be refused, got %q", id, got)
		}
	}
	if r := CodeRanges["AUTO"]; r != [2]int{790, 829} {
		t.Errorf("AUTO range = %v", r)
	}
	// every named range sits inside one of the two blocks and none overlaps another
	for a, ra := range CodeRanges {
		if !inFabricBlock(ra[0]) || !inFabricBlock(ra[1]) {
			t.Errorf("%s %v is outside both blocks", a, ra)
		}
		for b, rb := range CodeRanges {
			if a < b && ra[0] <= rb[1] && rb[0] <= ra[1] {
				t.Errorf("%s %v overlaps %s %v", a, ra, b, rb)
			}
		}
	}
	if got := problems([]registration{codeRow("E800"), codeRow("E800")}); len(got) != 1 || !strings.Contains(got[0], "duplicate") {
		t.Errorf("a duplicate in the AUTO block must be reported: %v", got)
	}
}
