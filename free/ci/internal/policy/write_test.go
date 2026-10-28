package policy

// Purpose: tests of the policy write path (P7-TRUST-26): the role check runs first and a refusal writes
// nothing, a team row needs its team id, a no-origin pin is refused, a Secret key is masked in the result and
// the audit row, and a failed audit row rolls the policy row back.
// Inputs:  a real SQLite store and the git fixtures of helpers_test.go.
// Outputs: pass or fail; no network. Fake values only.
// Constraints: tests only; a missing prerequisite is a t.Fatal, never a skip.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func testWriter(t *testing.T) (*Writer, *store.Store, roles.Actor) {
	t.Helper()
	st := testStore(t)
	svc := roles.New(st, &audit.Log{St: st})
	keys := testKeys(t)
	return &Writer{Store: st, Log: svc.Log, Keys: keys, Run: testRunner}, st, svc.Operator("cli:alice")
}

func rowCount(t *testing.T, st *store.Store, scope, subject string) int {
	t.Helper()
	rows, err := st.PolicyRows(context.Background(), scope, subject)
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

func TestWriterAuthorizeRunsFirst(t *testing.T) {
	w, st, actor := testWriter(t)
	denied := &roles.Error{Reason: roles.ReasonRole, Msg: "stub"}
	w.Authorize = func(context.Context, roles.Actor, roles.Role, string) error { return denied }
	for _, c := range []Change{
		{Op: OpSet, Scope: "user", Key: "trust.network", Value: json.RawMessage(`"none"`)},
		{Op: OpUnset, Scope: "user", Key: "trust.network"},
		{Op: OpPin, SHA: strings.Repeat("a", 40), Project: "/p"},
		{Op: OpClear, Project: "/p"},
	} {
		if _, err := w.Apply(context.Background(), actor, c); !errors.Is(err, denied) {
			t.Fatalf("%+v: want the Authorize error, got %v", c, err)
		}
	}
	if rowCount(t, st, "user", LocalSubject) != 0 {
		t.Fatal("a refused write left a row")
	}
	entries, _ := st.AuditEntries(context.Background(), 0, 100)
	if len(entries) != 0 {
		t.Fatalf("the writer adds no audit row of its own before Authorize passes: %d", len(entries))
	}
}

func TestWriterScopesAndSubjects(t *testing.T) {
	w, st, actor := testWriter(t)
	ctx := context.Background()
	if _, err := w.Apply(ctx, actor, Change{Op: OpSet, Scope: "team", Key: "trust.network", Value: json.RawMessage(`"none"`)}); err == nil || !strings.HasPrefix(err.Error(), "E651") {
		t.Fatalf("a team write without a team id is E651, got %v", err)
	}
	if _, err := w.Apply(ctx, actor, Change{Op: OpSet, Scope: "project", Key: "trust.network", Value: json.RawMessage(`"none"`)}); err == nil || !strings.HasPrefix(err.Error(), "E651") {
		t.Fatalf("a project write without a project is E651, got %v", err)
	}
	res, err := w.Apply(ctx, actor, Change{Op: OpSet, Scope: "team", Team: "t1", Project: "/p", Key: "trust.network", Value: json.RawMessage(` "none" `)})
	if err != nil || res.Subject != "t1" || res.Value != `"none"` {
		t.Fatalf("team write: %+v %v", res, err)
	}
	if rowCount(t, st, "team", "t1") != 1 || rowCount(t, st, "user", LocalSubject) != 0 {
		t.Fatal("the row lands in the team scope only")
	}
	// The project row may not loosen the team row: the pair is validated together.
	if _, err := w.Apply(ctx, actor, Change{Op: OpSet, Scope: "project", Team: "t1", Project: "/p", Key: "trust.network", Value: json.RawMessage(`"internet"`)}); err == nil || !strings.HasPrefix(err.Error(), "E670") {
		t.Fatalf("a project row looser than its team row is E670, got %v", err)
	}
}

func TestWriterSecretKeyIsMasked(t *testing.T) {
	w, st, actor := testWriter(t)
	if err := w.Keys.Register(KeySpec{Key: "test.token", Owner: "test", Kind: TightenAny, Secret: true,
		Decode: func(raw json.RawMessage) (any, error) { var s string; return s, json.Unmarshal(raw, &s) }}); err != nil {
		t.Fatal(err)
	}
	const secret = "fake-secret-value-0002"
	res, err := w.Apply(context.Background(), actor, Change{Op: OpSet, Scope: "user", Key: "test.token", Value: json.RawMessage(`"` + secret + `"`)})
	if err != nil || res.Value != Masked || strings.Contains(res.Key+res.Value, secret) {
		t.Fatalf("result must be masked: %+v %v", res, err)
	}
	entries, _ := st.AuditEntries(context.Background(), 0, 100)
	var set []store.AuditEntry
	for _, e := range entries {
		if e.Action == "policy.set" {
			set = append(set, e)
		}
	}
	if len(set) != 1 || strings.Contains(set[0].Detail, secret) || !strings.Contains(set[0].Detail, Masked) {
		t.Fatalf("one policy.set audit row without the value: %+v", set)
	}
}

func TestWriterPinNeedsAnOrigin(t *testing.T) {
	w, st, actor := testWriter(t)
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	write(t, dir+"/f", "x")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "c")
	sha := git(t, dir, "rev-parse", "HEAD")
	_, err := w.Apply(context.Background(), actor, Change{Op: OpPin, SHA: sha, Root: dir, Project: dir})
	if err == nil || !strings.HasPrefix(err.Error(), "E672") {
		t.Fatalf("a pin with no origin to check against is E672, got %v", err)
	}
	if _, ok, _ := st.PolicyPinFor(context.Background(), dir); ok {
		t.Fatal("a refused pin wrote a row")
	}
}

func TestWriterAuditFailureRollsTheRowBack(t *testing.T) {
	w, st, actor := testWriter(t)
	audit.SetScanner(blobScanner("fake-registered-value"))
	t.Cleanup(func() { audit.SetScanner(nil) })
	_, err := w.Apply(context.Background(), actor, Change{Op: OpSet, Scope: "user", Key: "trust.providers", Value: json.RawMessage(`["fake-registered-value"]`)})
	if err == nil || rowCount(t, st, "user", LocalSubject) != 0 {
		t.Fatalf("the audit refusal must roll the row back: err %v", err)
	}
}

// blobScanner flags every occurrence of one fake value, so an audit row that holds it is refused.
type blobScanner string

func (b blobScanner) Scan(blob []byte) []redact.Match {
	if i := bytes.Index(blob, []byte(b)); i >= 0 {
		return []redact.Match{{Offset: i, Len: len(b)}}
	}
	return nil
}

func TestConcurrentWritersRejectStalePolicy(t *testing.T) {
	w, st, actor := testWriter(t)
	var absent json.RawMessage
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, value := range []string{`1`, `2`} {
		go func(value string) {
			other := *w
			<-start
			_, err := other.Apply(context.Background(), actor, Change{Op: OpSet, Scope: "user", Key: "sched.test", Value: json.RawMessage(value), Expect: &absent})
			results <- err
		}(value)
	}
	close(start)
	successes, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, store.ErrPolicyConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	entries, err := st.AuditEntries(context.Background(), 0, 100)
	writes := 0
	for _, entry := range entries {
		if entry.Action == "policy.set" {
			writes++
		}
	}
	if err != nil || writes != 1 {
		t.Fatalf("policy audit writes=%d err=%v", writes, err)
	}
}
