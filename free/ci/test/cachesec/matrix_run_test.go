//go:build cachesec

package cachesec

// Purpose: TestCacheSecMatrix runs every case of matrix.yaml against the real stores and engines through
//          the rig, compares what it observed with the expected outcome, code, reason key and audit row, and
//          writes testdata/results.json (one row per case). It fails when a case is unmapped, when a threat
//          row this suite owns has no case, when an outcome differs from the expected one, and when a refusal
//          lacks its code, reason key or audit row.
// Inputs:  matrix.yaml (JSON, a subset of YAML) and the case runners in the other test files.
// Outputs: testdata/results.json; pass or fail.
// Constraints: a runner returns nil when the action was admitted and the refusal otherwise; the runners
//          assert that no bytes landed. Nothing is relaxed here to make a case pass: a case that does not
//          match its expected row is reported as it was observed.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// ownedRows are the threat-model rows (hq architecture/ci-threat-model.md) this suite must cover.
var ownedRows = []string{"TM-12", "TM-13", "TM-14", "TM-15", "TM-16", "TM-17", "TM-18", "TM-20", "TM-24"}

// refusal is a refusal reported as data (a verifier result, a delegated test).
type refusal struct{ Code, Reason, Msg string }

func (e *refusal) Error() string { return e.Code + " " + e.Reason + ": " + e.Msg }

// Row is one line of testdata/results.json.
type Row struct {
	ID         string `json:"id"`
	ThreatRow  string `json:"threat_row"`
	Expected   string `json:"expected"`
	Outcome    string `json:"outcome"`
	Code       string `json:"code"`
	Reason     string `json:"reason"`
	Audit      string `json:"audit"`
	AuditFound bool   `json:"audit_found"`
	Match      bool   `json:"match"`
	Platform   string `json:"platform"`
	Note       string `json:"note,omitempty"`
}

var codeRe = regexp.MustCompile(`\bE[0-9]{3}\b`)

// observe reads the code and reason key off a refusal. The reason is the Reason field of the first error
// in the chain that has one.
func observe(err error) (code, reason string) {
	if err == nil {
		return "", ""
	}
	code = codeRe.FindString(err.Error())
	for e := err; e != nil; e = errors.Unwrap(e) {
		v := reflect.ValueOf(e)
		for v.Kind() == reflect.Pointer && !v.IsNil() {
			v = v.Elem()
		}
		if v.Kind() == reflect.Struct {
			if f := v.FieldByName("Reason"); f.IsValid() && f.Kind() == reflect.String && f.String() != "" {
				return code, f.String()
			}
		}
	}
	return code, ""
}

// run dispatches a case to the runner of its family.
func (r *rig) run(id string) (error, string) {
	switch id {
	case "fork-main", "pr-a-pr-b", "field-boundary-collision", "ended-local", "epoch-local":
		return r.cacheWrite(id), ""
	case "ended-ssh", "epoch-ssh", "ended-agent", "epoch-agent":
		return r.remoteWrite(id), ""
	case "restore-traversal", "restore-absolute", "restore-symlink", "restore-hardlink", "restore-fifo", "restore-device",
		"restore-ratio", "secret-snapshot", "corrupt-local", "partial-local":
		err := r.restore(id)
		return err, r.note
	case "cache-secret-raw", "cache-secret-base64", "artifact-secret-raw", "artifact-secret-base64":
		return r.secret(id), ""
	case "artifact-overwrite", "artifact-substitution":
		return r.artifactMutation(id), ""
	case "get-cwd", "get-nonempty", "get-hooks":
		return r.getDest(id), ""
	case "corrupt-transit", "partial-transit":
		return r.transferBlob(id), ""
	case "corrupt-readthrough", "partial-readthrough":
		return r.readThrough(id), ""
	case "release-no-deps", "release-signed-remote", "release-revoked-remote", "untrusted-promotion":
		return r.release(id), ""
	case "manifest-plugins-key", "manifest-agent-key":
		return r.manifestKey(id), ""
	case "sign-uncommitted":
		return r.signUncommitted()
	case "team-member-inputs", "job-signer":
		return r.roleRefusal(id), ""
	}
	r.t.Fatalf("matrix case %q has no runner", id)
	return nil, ""
}

func TestCacheSecMatrix(t *testing.T) {
	cases := loadMatrix(t)
	covered := map[string]int{}
	for _, c := range cases {
		covered[c.ThreatRow]++
	}
	for _, row := range ownedRows {
		if covered[row] == 0 {
			t.Errorf("threat row %s has no case", row)
		}
	}
	var rows []Row
	for _, c := range cases {
		var row Row
		ok := t.Run(c.ID, func(t *testing.T) {
			r := newRig(t)
			err, note := r.run(c.ID)
			row = Row{ID: c.ID, ThreatRow: c.ThreatRow, Expected: c.Expected, Audit: c.Audit, Platform: runtime.GOOS + "/" + runtime.GOARCH, Note: note}
			if err == nil {
				row.Outcome = "admitted"
			} else {
				row.Outcome = "refused"
				row.Code, row.Reason = observe(err)
				if row.Reason == "" && c.Reason != "" && strings.Contains(err.Error(), c.Reason) {
					row.Reason, row.Note = c.Reason, "reason key read from the error text (the error type has no Reason field)"
				}
			}
			if c.Expected == "refused" {
				row.AuditFound = r.audit(c.Audit, c.Reason)
			}
			row.Match = row.Outcome == c.Expected
			if c.Expected == "refused" && row.Outcome == "refused" {
				row.Match = row.Code == c.Code && row.Reason == c.Reason && row.AuditFound
			}
			if !row.Match {
				t.Errorf("%s: expected %s %s %s audit %q; observed %s %s %s audit found %v (%v)", c.ID, c.Expected, c.Code, c.Reason, c.Audit,
					row.Outcome, row.Code, row.Reason, row.AuditFound, err)
			}
		})
		if row.ID == "" { // the runner stopped the subtest before it recorded anything
			row = Row{ID: c.ID, ThreatRow: c.ThreatRow, Expected: c.Expected, Audit: c.Audit, Outcome: "error", Platform: runtime.GOOS + "/" + runtime.GOARCH}
		}
		if !ok {
			row.Match = false
		}
		rows = append(rows, row)
	}
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("testdata/results.json", append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := 0
	for _, row := range rows {
		if !row.Match {
			bad++
		}
	}
	if bad != 0 {
		t.Errorf("%d of %d cases differ from the matrix", bad, len(rows))
	}
	t.Log(fmt.Sprintf("%d cases, %d match", len(rows), len(rows)-bad))
}
