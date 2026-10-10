package contract

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureBytes(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "http-full", "routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// withField sets one extra key at the given depth of the first route's first location.
func withField(t *testing.T, where, key string) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(fixtureBytes(t), &doc); err != nil {
		t.Fatal(err)
	}
	route := doc["routes"].([]any)[0].(map[string]any)
	switch where {
	case "top":
		doc[key] = true
	case "route":
		route[key] = true
	case "location":
		route["locations"].([]any)[0].(map[string]any)[key] = []string{"10.0.0.0/8"}
	case "defaults":
		doc["defaults"].(map[string]any)[key] = 1
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLoadAcceptsTheFixtures(t *testing.T) {
	if _, err := Load(bytes.NewReader(fixtureBytes(t))); err != nil {
		t.Fatal(err)
	}
}

// TestLoadFailsClosedOnUnknownFields: a future restriction field must be a
// refusal naming the field, never dropped (it would render as an unrestricted route).
func TestLoadFailsClosedOnUnknownFields(t *testing.T) {
	for _, c := range []struct{ where, key string }{
		{"top", "future_top"}, {"route", "future_route"}, {"location", "allow_cidrs"}, {"defaults", "future_default"},
	} {
		m, err := Load(bytes.NewReader(withField(t, c.where, c.key)))
		if err == nil || m != nil || !strings.Contains(err.Error(), c.key) {
			t.Errorf("%s.%s: want an error naming the field, got %v", c.where, c.key, err)
		}
	}
}

func TestLoadRejectsTrailingData(t *testing.T) {
	for _, tail := range []string{`{"schema_version":"1"}`, `garbage`, `[1]`} {
		b := append(append([]byte{}, fixtureBytes(t)...), []byte("\n"+tail)...)
		if _, err := Load(bytes.NewReader(b)); err == nil || !strings.Contains(err.Error(), "trailing") {
			t.Errorf("trailing %q: got %v", tail, err)
		}
	}
}
