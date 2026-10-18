package hygiene

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
)

// fakeRuntime is an in-memory container runtime: resources are "kind/id" labelled with an attempt.
type fakeRuntime struct {
	mu      sync.Mutex
	items   map[string]string // "kind/id" -> attempt label ("" = unlabelled)
	failRm  string            // id whose removal fails
	failLs  bool
	down    bool // installed but not answering
	removed []string
}

func newFakeRuntime() *fakeRuntime { return &fakeRuntime{items: map[string]string{}} }

func (f *fakeRuntime) add(kind exec.ResourceKind, id, attempt string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[string(kind)+"/"+id] = attempt
}

func (f *fakeRuntime) List(_ context.Context, kind exec.ResourceKind, attempt string) ([]exec.Labelled, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failLs {
		return nil, errors.New("daemon error")
	}
	if f.down {
		return nil, &RuntimeDownError{Binaries: []string{"docker"}}
	}
	var out []exec.Labelled
	for k, a := range f.items {
		id, ok := strings.CutPrefix(k, string(kind)+"/")
		if !ok || a == "" || (attempt != "" && a != attempt) {
			continue // unlabelled resources are invisible to hygiene
		}
		out = append(out, exec.Labelled{ID: id, Attempt: a})
	}
	return out, nil
}

func (f *fakeRuntime) Remove(_ context.Context, kind exec.ResourceKind, id, attempt string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id == f.failRm {
		return fmt.Errorf("cannot remove %s", id)
	}
	if f.items[string(kind)+"/"+id] != attempt {
		return nil // relabelled or replaced since the listing: not ours any more
	}
	delete(f.items, string(kind)+"/"+id)
	f.removed = append(f.removed, string(kind)+"/"+id)
	return nil
}

func (f *fakeRuntime) has(kind exec.ResourceKind, id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.items[string(kind)+"/"+id]
	return ok
}

// testHost returns a Host over a fresh temp base with a fake runtime.
func testHost(t *testing.T) (*Host, *fakeRuntime) {
	t.Helper()
	home := t.TempDir()
	rt := newFakeRuntime()
	return &Host{Base: home + "/work", Home: home, Runtime: rt, ScanDirs: []string{}}, rt
}

func codeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}
