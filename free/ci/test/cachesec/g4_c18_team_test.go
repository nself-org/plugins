//go:build cachesec

package cachesec

// Purpose: TM-18 cases on the coordinator carrier (a TEAM member fetching release inputs). The routes are the
// real remote.Handlers behind the real user API; every refusal is read from the wire body (clientapi.Body).
// Inputs: newWorld, newG4Team. Outputs: registered cases C18-TEAM-01 and C18-TEAM-02.
// Constraints: production packages only; the audit action is read from the store's audit table.

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/release/remote"
	"github.com/nself-org/plugins/free/ci/internal/roles"
)

// g4Leaked is the number of release bytes (the binary, its digest path, the records) found in a refusal body.
func g4Leaked(tm *g4Team, body []byte) int64 {
	var n int64
	for _, needle := range []string{g4TeamBytes, tm.app, `"records"`, `"evidence"`} {
		if bytes.Contains(body, []byte(needle)) {
			n += int64(len(body))
		}
	}
	return n
}

// g4Has returns action when the store's audit table holds a row with it, "" otherwise.
func g4Has(w *world, action string) string {
	if len(g4AuditRows(w, action)) == 0 {
		return ""
	}
	return action
}

// g4DenyRow finds the audit row "authorize" whose detail says result=deny with the reason.
func g4DenyRow(w *world, reason string) bool {
	for _, r := range g4AuditRows(w, "authorize") {
		if strings.Contains(r.Detail, "result=deny") && strings.Contains(r.Detail, "reason="+reason) {
			return true
		}
	}
	return false
}

func init() {
	register("C18-TEAM-01", func(t *testing.T) Observed {
		w := newWorld(t)
		tm := newG4Team(w)
		// positive control: a release-role holder of the project reads the inputs and the blob.
		releaser := tm.client(roles.Release, g4TeamProject)
		status, raw, _ := tm.get(releaser, remote.InputsPath(g4TeamRun))
		if status != http.StatusOK || !strings.Contains(string(raw), remote.Schema) {
			t.Fatalf("control: the release holder was refused: %d %s", status, raw)
		}
		if status, raw, _ = tm.get(releaser, remote.BlobPath(g4TeamRun, tm.app)); status != http.StatusOK || string(raw) != g4TeamBytes {
			t.Fatalf("control: the release holder cannot fetch the blob: %d", status)
		}
		// attack: a TEAM member (below release) asks for the same inputs and the same blob.
		member := tm.client(roles.Member, g4TeamProject)
		var leaked int64
		var first string
		for _, path := range []string{remote.InputsPath(g4TeamRun), remote.BlobPath(g4TeamRun, tm.app)} {
			status, raw, body := tm.get(member, path)
			if status != http.StatusForbidden || body.Code != roles.CodeNotAuthorized {
				t.Fatalf("%s: member got %d %s", path, status, raw)
			}
			leaked += g4Leaked(tm, raw)
			first = body.Reason
		}
		if !g4DenyRow(w, roles.ReasonRole) {
			t.Fatalf("no audited deny row for the member: %v", g4AuditRows(w, "authorize"))
		}
		return Observed{Outcome: Refused, Code: roles.CodeNotAuthorized, Reason: first, Audit: g4Has(w, "authorize"), Landed: leaked}
	})

	register("C18-TEAM-02", func(t *testing.T) Observed {
		w := newWorld(t)
		tm := newG4Team(w)
		owner := tm.client(roles.Release, g4TeamProject)
		if status, _, _ := tm.get(owner, remote.InputsPath(g4TeamRun)); status != http.StatusOK { // positive control
			t.Fatalf("control: the owning project's release holder was refused: %d", status)
		}
		// the body of "no such run" is the yardstick every other refusal must equal byte for byte.
		status, want, wantBody := tm.get(owner, remote.InputsPath("run-nope"))
		if status != http.StatusNotFound || wantBody.Code != records.CodeNotFound {
			t.Fatalf("unknown run: %d %s", status, want)
		}
		other := tm.client(roles.Release, "other/app") // holds release, but on another project
		var leaked int64
		for _, path := range []string{remote.InputsPath(g4TeamRun), remote.BlobPath(g4TeamRun, tm.app)} {
			status, raw, body := tm.get(other, path)
			if status != http.StatusNotFound || !bytes.Equal(raw, want) || body.Code != records.CodeNotFound {
				t.Fatalf("%s: foreign release holder got %d %s, want 404 %s", path, status, raw, want)
			}
			leaked += g4Leaked(tm, raw)
		}
		if !g4DenyRow(w, roles.ReasonProject) {
			t.Fatalf("the project refusal is not audited: %v", g4AuditRows(w, "authorize"))
		}
		return Observed{Outcome: Refused, Code: wantBody.Code, Reason: wantBody.Reason, Audit: g4Has(w, "authorize"), Landed: leaked}
	})
}
