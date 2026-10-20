//go:build cachesec

package cachesec

// Purpose: TestCacheSecMatrix runs every case of matrix.yaml against its expected outcome and writes
// testdata/results.json (P7-CACHE-11). It fails when a case has no function, a function has no row, a
// row names no threat-model row, a threat row of the suite's range (TM-12 to TM-18) has no case, a
// refusal lacks its code, reason key or audit row, or a refusal let bytes land. Expected outcomes: refused (a coded
// refusal), miss (a read that returns nothing and carries no fabric code, e.g. a cache lookup of another namespace),
// admitted (the positive release case). Audit "none" marks a refusal that production does not audit yet: the row
// names its debt id in `gap`, and the case asserts that no audit row appeared.
// Inputs:  matrix.yaml; $HQ (the hq checkout) to check the TM ids against architecture/ci-threat-model.md;
//          INTEGRATION=1 (the remote cases start the fabric sim and fail, never skip, without it).
// Outputs: testdata/results.json, one row per case: id, threat, carrier, expected, outcome, code, reason,
//          audit, landed_bytes. The file is rewritten on every run; the order is the matrix order.
// Constraints: the results file holds no time, host or path, so a rerun on the same tree is byte equal.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type matrixCase struct {
	ID       string `yaml:"id"`
	Threat   string `yaml:"threat"`
	Carrier  string `yaml:"carrier"`
	Title    string `yaml:"title"`
	Expected string `yaml:"expected"`
	Code     string `yaml:"code"`
	Reason   string `yaml:"reason"`
	Audit    string `yaml:"audit"`
	Gap      string `yaml:"gap"`
}

type matrixFile struct {
	Version int          `yaml:"version"`
	Cases   []matrixCase `yaml:"cases"`
}

// resultRow is one row of testdata/results.json.
type resultRow struct {
	ID      string `json:"id"`
	Threat  string `json:"threat"`
	Carrier string `json:"carrier"`
	Expect  string `json:"expected"`
	Outcome string `json:"outcome"`
	Code    string `json:"code"`
	Reason  string `json:"reason"`
	Audit   string `json:"audit"`
	Landed  int64  `json:"landed_bytes"`
}

// suiteThreats are the threat-model rows this suite must cover (hq architecture/ci-threat-model.md).
var suiteThreats = []string{"TM-12", "TM-13", "TM-14", "TM-15", "TM-16", "TM-17", "TM-18"}

var (
	caseIDPattern = regexp.MustCompile(`^C(1[2-8])-[A-Z0-9]+-\d{2}$`)
	codePattern   = regexp.MustCompile(`^E\d{3}$`)
	reasonPattern = regexp.MustCompile(`^[a-z0-9_]+$`)
	gapPattern    = regexp.MustCompile(`^D-\d{4}$`)
	carriers      = map[string]bool{"local": true, "ssh": true, "agent": true, "coordinator": true, "signer": true}
)

func loadMatrix(t *testing.T) matrixFile {
	t.Helper()
	raw, err := os.ReadFile("matrix.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var m matrixFile
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err = dec.Decode(&m); err != nil {
		t.Fatalf("matrix.yaml: %v", err)
	}
	if m.Version != 1 || len(m.Cases) == 0 {
		t.Fatalf("matrix.yaml: version %d, %d cases", m.Version, len(m.Cases))
	}
	return m
}

// TestMatrixShape is the static half: it needs no store and no Docker.
func TestMatrixShape(t *testing.T) {
	m := loadMatrix(t)
	ids := map[string]bool{}
	covered := map[string]bool{}
	for _, c := range m.Cases {
		if !caseIDPattern.MatchString(c.ID) || ids[c.ID] {
			t.Errorf("case id %q is malformed or repeated", c.ID)
		}
		ids[c.ID] = true
		if !strings.HasPrefix(c.ID, "C"+strings.TrimPrefix(c.Threat, "TM-")+"-") {
			t.Errorf("%s: id does not start with its threat row %s", c.ID, c.Threat)
		}
		covered[c.Threat] = true
		if c.Title == "" || !carriers[c.Carrier] {
			t.Errorf("%s: title empty or carrier %q unknown", c.ID, c.Carrier)
		}
		switch c.Expected {
		case Refused:
			if !codePattern.MatchString(c.Code) || !reasonPattern.MatchString(c.Reason) || c.Audit == "" {
				t.Errorf("%s: a refusal needs a code, a reason key and an audit action", c.ID)
			}
			// audit: none is a known gap of production, never a choice: it names the open debt row that owns the missing audit row.
			if (c.Audit == "none") != (c.Gap != "") || (c.Gap != "" && !gapPattern.MatchString(c.Gap)) {
				t.Errorf("%s: audit none needs gap: D-nnnn (the debt row), and a gap needs audit none", c.ID)
			}
		case Miss:
			if c.Code != "" || !reasonPattern.MatchString(c.Reason) || (c.Audit == "none") != (c.Gap != "") {
				t.Errorf("%s: a miss carries a reason key and no code, and audit none only with a gap", c.ID)
			}
		case Admitted:
			if c.Code != "" || c.Reason != "" {
				t.Errorf("%s: an admitted case carries no code or reason", c.ID)
			}
		default:
			t.Errorf("%s: expected must be %s, %s or %s", c.ID, Refused, Miss, Admitted)
		}
		if registry[c.ID] == nil {
			t.Errorf("%s: no function registered for this case", c.ID)
		}
	}
	for _, id := range registered() {
		if !ids[id] {
			t.Errorf("function %s is registered but has no matrix row", id)
		}
	}
	for _, th := range suiteThreats {
		if !covered[th] {
			t.Errorf("threat row %s has no case", th)
		}
	}
	for th := range covered {
		found := false
		for _, want := range suiteThreats {
			found = found || th == want
		}
		if !found {
			t.Errorf("case threat %s is outside the suite's range %v", th, suiteThreats)
		}
	}
	if hq := os.Getenv("HQ"); hq != "" {
		raw, err := os.ReadFile(filepath.Join(hq, "architecture", "ci-threat-model.md"))
		if err != nil {
			t.Fatalf("$HQ is set but the threat model cannot be read: %v", err)
		}
		for _, th := range suiteThreats {
			if !regexp.MustCompile(`(?m)^\| ` + th + ` \|`).Match(raw) {
				t.Errorf("%s is not a row of the threat model", th)
			}
		}
	}
}

// TestCacheSecMatrix runs every case and writes testdata/results.json.
func TestCacheSecMatrix(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Fatal("INTEGRATION=1 is required: the remote cases start the fabric sim and a missing prerequisite is a failure, not a skip")
	}
	m := loadMatrix(t)
	rows := make([]resultRow, 0, len(m.Cases))
	for _, c := range m.Cases {
		fn := registry[c.ID]
		if fn == nil {
			t.Errorf("%s: no function registered", c.ID)
			continue
		}
		row := resultRow{ID: c.ID, Threat: c.Threat, Carrier: c.Carrier, Expect: c.Expected}
		t.Run(c.ID, func(t *testing.T) {
			got := fn(t)
			row.Outcome, row.Code, row.Reason, row.Audit, row.Landed = got.Outcome, got.Code, got.Reason, got.Audit, got.Landed
			if got.Outcome != c.Expected {
				t.Errorf("outcome %q, expected %q", got.Outcome, c.Expected)
			}
			if c.Expected == Miss {
				if got.Code != "" || got.Reason != c.Reason || got.Audit != c.Audit || got.Landed != 0 {
					t.Errorf("miss %s/%s audit %q landed %d, expected reason %s audit %q landed 0", got.Code, got.Reason, got.Audit, got.Landed, c.Reason, c.Audit)
				}
			}
			if c.Expected == Refused {
				if got.Code != c.Code || got.Reason != c.Reason {
					t.Errorf("refused with %s/%s, expected %s/%s", got.Code, got.Reason, c.Code, c.Reason)
				}
				if got.Audit != c.Audit {
					t.Errorf("audit row %q, expected %q", got.Audit, c.Audit)
				}
				if got.Landed != 0 {
					t.Errorf("%d bytes landed in the attacked store", got.Landed)
				}
			}
		})
		rows = append(rows, row)
	}
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join("testdata", "results.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
