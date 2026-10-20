//go:build cachesec

package cachesec

// Purpose: shared fixtures of group g1 (local cache stores: entries, specs, authz, store) for the
// P7-CACHE-11 suite: the three namespaces every g1 case uses, reason extractors for the typed
// refusals, and the helpers that make a case's positive control one line.
// Inputs: a world (framework_test.go). Outputs: g1 facts, g1ReasonE711/E684/E683, g1Key, g1Commit.
// Constraints: production packages only; reason keys that carry a dot in production ("cache.namespace",
// "path.traversal") are reported with the dot replaced by an underscore, because the matrix reason
// pattern is [a-z0-9_]+ (g1Key is the only place that rewrites a key).

import (
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

var (
	g1Main = entries.Facts{Project: "proj", Trust: "owner", Protection: "protected"}
	g1PR1  = entries.Facts{Project: "proj", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork/example", PR: "1"}
	g1PR2  = entries.Facts{Project: "proj", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork/example", PR: "2"}
)

// g1Key turns a production reason key into the matrix spelling.
func g1Key(reason string) string { return strings.ReplaceAll(reason, ".", "_") }

// g1ReasonE711 extracts the reason of an *entries.Refused (E711).
func g1ReasonE711(t *testing.T) func(error) string {
	return func(err error) string {
		var r *entries.Refused
		if !errors.As(err, &r) {
			t.Fatalf("want E711 *entries.Refused, got %T: %v", err, err)
		}
		return g1Key(r.Reason)
	}
}

// g1Denied is the E684 refusal of a cache write. authz.Denied has no Code method (only the constant
// authz.CodeCacheWrite and the "E684" prefix of its text), so the framework's refusal() cannot be
// used; the code reported is the production constant, and the text prefix is asserted too.
func g1Denied(t *testing.T, err error) Observed {
	t.Helper()
	var d *authz.Denied
	if !errors.As(err, &d) {
		t.Fatalf("want E684 *authz.Denied, got %T: %v", err, err)
	}
	if !strings.HasPrefix(err.Error(), authz.CodeCacheWrite) {
		t.Fatalf("the error text does not lead with %s: %v", authz.CodeCacheWrite, err)
	}
	return Observed{Outcome: Refused, Code: authz.CodeCacheWrite, Reason: g1Key(string(d.Reason))}
}

// g1PathRefused is the E683 refusal of an archive entry (authz.PathError, same remark as g1Denied).
func g1PathRefused(t *testing.T, err error) Observed {
	t.Helper()
	var p *authz.PathError
	if !errors.As(err, &p) {
		t.Fatalf("want E683 *authz.PathError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), authz.CodeArtifactPath) {
		t.Fatalf("the error text does not carry %s: %v", authz.CodeArtifactPath, err)
	}
	return Observed{Outcome: Refused, Code: authz.CodeArtifactPath, Reason: g1Key(string(p.Reason))}
}

// g1Commit commits one freshly Put blob of content under key into the namespace of f.
func (w *world) g1Commit(attempt string, epoch int64, f entries.Facts, key, content string) error {
	w.t.Helper()
	return w.Es.Commit(w.ctx, attempt, epoch, w.NS(f), w.Manifest(f, key, content))
}

// g1Honest runs a whole passing attempt of f and commits one entry; the positive control of a
// case whose attack needs a different attempt. It returns the key.
func (w *world) g1Honest(id string, f entries.Facts, extra string) string {
	w.t.Helper()
	key := w.Key(f, extra)
	fin := w.Attempt(id, f, []string{key}, true)
	if err := w.g1Commit(id, fin, f, key, "honest bytes of "+id); err != nil {
		w.t.Fatalf("positive control %s: the honest commit was refused: %v", id, err)
	}
	return key
}

// g1Visible is the number of entries stored under key in any namespace.
func (w *world) g1Visible(key string) int {
	w.t.Helper()
	list, err := w.St.CacheEntries(w.ctx, key)
	if err != nil {
		w.t.Fatal(err)
	}
	return len(list)
}

// g1Transition moves attempt id between states at epoch.
func (w *world) g1Transition(id, from, to string, epoch int64) {
	w.t.Helper()
	if err := w.St.Transition(w.ctx, id, model.JobState(from), model.JobState(to), store.TransitionDetail{Epoch: epoch}); err != nil {
		w.t.Fatalf("transition %s %s->%s: %v", id, from, to, err)
	}
}
