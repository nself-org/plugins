package compatcheck

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCompatCheck(t *testing.T) {
	b, err := os.ReadFile("../../plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Requires struct {
			Nself string `json:"nself"`
		} `json:"requires"`
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Requires.Nself != RequiresNself {
		t.Fatalf("manifest %q, constant %q", manifest.Requires.Nself, RequiresNself)
	}
	for _, c := range []struct {
		version string
		reject  bool
	}{{"", false}, {"1.4.11", true}, {"1.4.12", false}, {"1.5.0", false}, {"v1.5.0-rc.1", false}} {
		err := Check(func(string) string { return c.version }, RequiresNself)
		if c.reject && (err == nil || !strings.Contains(err.Error(), "E600") || !strings.Contains(err.Error(), "nself update")) {
			t.Errorf("%q: %v", c.version, err)
		}
		if !c.reject && err != nil {
			t.Errorf("%q: %v", c.version, err)
		}
	}
}
