package policy

// Purpose: regression tests for D-1283: the loosening check and the merge check of a policy write run
// inside the write transaction, so a concurrent writer cannot slip a row in between the check and the write.
// Inputs:  a real SQLite store, the git fixtures of helpers_test.go and a Runner that lands a concurrent
//          write the first time the writer calls git (which the old code did after its reads).
// Outputs: pass or fail; no network. Fake values only.
// Constraints: tests only; a missing prerequisite is a t.Fatal, never a skip.

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// racingRunner runs fn once, before its first git call, to stand in for a second writer
// that commits while the first writer is between its checks and its write.
func racingRunner(fn func()) Runner {
	var once sync.Once
	return func(ctx context.Context, argv ...string) (string, int, error) {
		once.Do(fn)
		return testRunner(ctx, argv...)
	}
}

func storedValue(t *testing.T, st *store.Store, scope, subject, key string) string {
	t.Helper()
	rows, err := st.PolicyRows(context.Background(), scope, subject)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Key == key {
			return string(r.Value)
		}
	}
	return ""
}

func TestMachineWriteCannotLandOverConcurrentTightening(t *testing.T) {
	f := newFixture(t)
	w, st, actor := testWriter(t)
	w.Machine = true
	put(t, st, "user", LocalSubject, "trust.isolation_min", `"sandboxed-container"`)
	w.Run = racingRunner(func() { put(t, st, "user", LocalSubject, "trust.isolation_min", `"ephemeral-vm"`) })
	// "vm" is stricter than the "sandboxed-container" the writer read, so the check passes; an operator
	// tightens to "ephemeral-vm" before the write lands, which makes it a loosening.
	_, err := w.Apply(context.Background(), actor, Change{Op: OpSet, Scope: "user", Key: "trust.isolation_min", Value: json.RawMessage(`"vm"`), Root: f.work, Project: "proj"})
	if err == nil || !strings.Contains(err.Error(), "machine_invoked") {
		t.Fatalf("want a machine_invoked refusal, got %v", err)
	}
	if got := storedValue(t, st, "user", LocalSubject, "trust.isolation_min"); got != `"ephemeral-vm"` {
		t.Fatalf("the machine write landed over the tightening: %s", got)
	}
}

func TestConcurrentTighteningsCannotBreakResolve(t *testing.T) {
	f := newFixture(t)
	w, st, actor := testWriter(t)
	put(t, st, "user", LocalSubject, "trust.isolation_min", `"sandboxed-container"`)
	w.Run = racingRunner(func() { put(t, st, "user", LocalSubject, "trust.isolation_min", `"ephemeral-vm"`) })
	// A project row of "vm" merges over the "sandboxed-container" user row the writer read, but not over the
	// "ephemeral-vm" that lands next: the project row would be looser than its parent and Resolve would fail.
	_, err := w.Apply(context.Background(), actor, Change{Op: OpSet, Scope: "project", Key: "trust.isolation_min", Value: json.RawMessage(`"vm"`), Root: f.work, Project: "proj"})
	if err == nil || !strings.Contains(err.Error(), "E670") {
		t.Fatalf("want E670, got %v", err)
	}
	if got := storedValue(t, st, "project", "proj", "trust.isolation_min"); got != "" {
		t.Fatalf("the looser project row was written: %s", got)
	}
}
