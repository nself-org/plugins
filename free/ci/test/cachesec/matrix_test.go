package cachesec

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
)

type matrixCase struct {
	ID        string `json:"id"`
	ThreatRow string `json:"threat_row"`
	Expected  string `json:"expected"`
	Code      string `json:"code"`
	Reason    string `json:"reason"`
	Audit     string `json:"audit"`
}

func loadMatrix(t *testing.T) []matrixCase {
	t.Helper()
	b, err := os.ReadFile("matrix.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []matrixCase
	if err = json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	row := regexp.MustCompile(`^TM-[0-9]{2}$`)
	for _, c := range cases {
		if c.ID == "" || seen[c.ID] || !row.MatchString(c.ThreatRow) {
			t.Fatalf("duplicate or unmapped case: %q", c.ID)
		}
		seen[c.ID] = true
		if c.Expected != "refused" && c.Expected != "admitted" {
			t.Fatalf("%s: missing expected outcome", c.ID)
		}
		if c.Expected == "refused" && (c.Code == "" || c.Reason == "" || c.Audit == "") {
			t.Fatalf("%s: incomplete refusal contract", c.ID)
		}
	}
	if len(cases) == 0 {
		t.Fatal("empty matrix")
	}
	return cases
}

// TestMatrixContract checks the manifest even when the drill tag is absent.
func TestMatrixContract(t *testing.T) { loadMatrix(t) }
