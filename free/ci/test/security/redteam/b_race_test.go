//go:build security

package redteam

// Purpose: race attacks (run once under -race): values registered while streams are being written
// (TM-29), and concurrent cache commits from a trusted and several untrusted attempts that all use
// the same key string (TM-12, TM-13).
// Inputs:  the real redact.Redactor and entries.Store over a temp sqlite store.
// Outputs: pass or fail; the race detector reports data races as failures.
// Constraints: deterministic outcome (assertions are on end state); finishes in seconds.

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

func TestB_Race_RegisterWhileStreaming(t *testing.T) {
	r := redact.New()
	registered := make(chan struct{})
	var wg sync.WaitGroup
	outs := make([]string, 4)
	for i := range outs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f := r.NewFilter()
			var out []byte
			for j := 0; j < 100; j++ {
				out = append(out, f.Redact([]byte(fmt.Sprintf("phase1 %d value-%02d-secret-xyz\n", j, j%5)))...)
			}
			<-registered // the value is registered while this stream is open
			for j := 0; j < 100; j++ {
				out = append(out, f.Redact([]byte(fmt.Sprintf("phase2 %d %s value-%02d-secret-xyz\n", j, fakeSecret, j%5)))...)
			}
			outs[i] = string(out) + string(f.Flush())
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 5; j++ {
			_ = r.Register([]byte(fmt.Sprintf("value-%02d-secret-xyz", j)))
		}
		_ = r.Register([]byte(fakeSecret))
		close(registered)
	}()
	wg.Wait()
	for i, o := range outs {
		for _, line := range strings.Split(o, "\n") {
			if strings.HasPrefix(line, "phase2") && (strings.Contains(line, fakeSecret) || strings.Contains(line, "-secret-xyz")) {
				t.Errorf("stream %d printed a value registered before the line was written: %q", i, line)
				break
			}
		}
	}
}

func TestB_Race_ConcurrentCommitsKeepNamespacesApart(t *testing.T) {
	e := newCacheEnv(t)
	facts := []entries.Facts{factsMain, factsPR1, factsPR2}
	const key = "same-key-string"
	keys := map[string]string{}
	for i, f := range facts {
		id := fmt.Sprintf("race%d", i)
		keys[id] = e.key(f, key)
		e.start(id, f)
		e.finish(id, f, []string{keys[id]}, true)
	}
	// every namespace uses a different derived key; also force the SAME 64-hex key string everywhere
	shared := e.key(factsMain, "shared")
	for i, f := range facts {
		id := fmt.Sprintf("shr%d", i)
		e.start(id, f)
		e.finish(id, f, []string{shared}, true)
	}
	var wg sync.WaitGroup
	for i, f := range facts {
		wg.Add(1)
		go func(i int, f entries.Facts) {
			defer wg.Done()
			id := fmt.Sprintf("shr%d", i)
			if err := e.es.Commit(e.ctx, id, 3, e.ns(f), e.manifest(f, f, shared, fmt.Sprintf("content-of-%d", i))); err != nil {
				t.Errorf("commit %s: %v", id, err)
			}
		}(i, f)
	}
	wg.Wait()
	for i, f := range facts {
		m, ok, err := e.es.Lookup(e.ctx, e.ns(f), shared, authz.ModeRWOwn)
		if err != nil || !ok {
			t.Fatalf("namespace %d lost its entry: %v", i, err)
		}
		want := e.manifest(f, f, shared, fmt.Sprintf("content-of-%d", i)).Blobs[0].SHA256
		if m.Blobs[0].SHA256 != want {
			t.Errorf("namespace %d serves another namespace's bytes", i)
		}
	}
}
