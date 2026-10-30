package projection

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const data = "../testdata"

func readData(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(data, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join(data, name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if want := readData(t, name); string(want) != string(got) {
		t.Errorf("%s drifted; run go test ./internal/evidence/projection -update\n got: %s", name, got)
	}
}

func projectJSON(t *testing.T, doc []byte) []byte {
	t.Helper()
	p, err := Project(doc, Name)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}

func TestProjectionGolden(t *testing.T) {
	golden(t, "projected.attempts.json", projectJSON(t, readData(t, "evidence.attempts.json")))
	golden(t, "Evidence-Projection.md", []byte(Page()))
	page := Page()
	for _, m := range Mappings() {
		if !strings.Contains(page, "| `"+m.Field+"` | `"+m.Source+"` |") {
			t.Errorf("page lacks %s <- %s", m.Field, m.Source)
		}
		if m.Source == "" {
			t.Errorf("field %s has no source key", m.Field)
		}
	}
	p, _ := Project(readData(t, "evidence.attempts.json"), Name)
	if _, err := Project(nil, "other/1"); err == nil {
		t.Error("an unknown projection must be refused")
	}
	rows := p.Fields["attempts"].Value.([]map[string]Value)
	if len(rows) != 3 || rows[1]["runner_kind"].Value != "ssh" || rows[2]["runner_kind"].Value != "hosted" {
		t.Errorf("attempt rows: %+v", rows)
	}
}

func TestProjectionMissingKeys(t *testing.T) {
	golden(t, "projected.pre-auto21.json", projectJSON(t, readData(t, "evidence.pre-auto21.json")))
	p, err := Project(readData(t, "evidence.pre-auto21.json"), Name)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"attempts", "errors"} {
		v := p.Fields[f]
		if v.Value != nil || !strings.HasPrefix(v.Reason, "source key absent") {
			t.Errorf("%s: %+v", f, v)
		}
	}
	if v := p.Fields["duration_ms"]; v.Value != nil || !strings.Contains(v.Reason, "null") {
		t.Errorf("duration_ms: %+v", v)
	}
	if v := p.Fields["result"]; v.Value != "pass" || v.Reason != "" {
		t.Errorf("result: %+v", v)
	}
	// an empty object and a non-object must not panic
	if _, err := Project([]byte(`{}`), Name); err != nil {
		t.Error(err)
	}
	if _, err := Project([]byte(`[]`), Name); err == nil {
		t.Error("a non-object must be an error")
	}
}
