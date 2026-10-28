package remote_test

// Purpose: signing a release of N blobs collects the release inputs once, not once per blob
// (D-1356); the entry expires with its TTL; the inputs route always collects afresh.
// Constraints: not parallel (shared route registry); a fake clock, no sleeps.

import (
	"bytes"
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/release/remote"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// countingStore counts the collects (one PipelineJobs read each) the handlers make.
type countingStore struct {
	*fakeStore
	collects atomic.Int64
}

func (c *countingStore) PipelineJobs(ctx context.Context, run string) ([]store.RunJob, error) {
	c.collects.Add(1)
	return c.fakeStore.PipelineJobs(ctx, run)
}

func TestBlobRequestsShareOneCollect(t *testing.T) {
	l := newLocalAPI(t)
	now := time.Unix(1000, 0)
	cs := &countingStore{fakeStore: l.fs}
	useDeps(t, &remote.Deps{Runs: fakeRuns{}, Store: cs, Blobs: l.blobs,
		Cache: &remote.InputsCache{TTL: 5 * time.Second, Now: func() time.Time { return now }}})
	blob := remote.BlobPath(uRun, digestOf(l.app))
	sbom := remote.BlobPath(uRun, digestOf([]byte("{}")))

	for i := 0; i < 5; i++ {
		for _, p := range []string{blob, sbom} {
			if resp, _ := l.get(t, p); resp.StatusCode != http.StatusOK {
				t.Fatalf("%s: %d", p, resp.StatusCode)
			}
		}
	}
	if n := cs.collects.Load(); n != 1 {
		t.Fatalf("10 blob requests collected %d times, want 1", n)
	}

	now = now.Add(6 * time.Second) // the entry is older than its TTL
	if resp, body := l.get(t, blob); resp.StatusCode != http.StatusOK || !bytes.Equal(body, l.app) {
		t.Fatalf("after expiry: %d %q", resp.StatusCode, body)
	}
	if n := cs.collects.Load(); n != 2 {
		t.Fatalf("after expiry collected %d times, want 2", n)
	}

	l.get(t, remote.InputsPath(uRun)) // the inputs route never reads the cache
	l.get(t, remote.InputsPath(uRun))
	if n := cs.collects.Load(); n != 4 {
		t.Fatalf("two inputs requests: %d collects, want 4", n)
	}
	l.get(t, blob) // and the list it collected answers the next blob
	if n := cs.collects.Load(); n != 4 {
		t.Fatalf("blob after inputs: %d collects, want 4", n)
	}
}

func TestBlobRouteWithoutACacheCollectsEveryTime(t *testing.T) {
	l := newLocalAPI(t)
	cs := &countingStore{fakeStore: l.fs}
	useDeps(t, &remote.Deps{Runs: fakeRuns{}, Store: cs, Blobs: l.blobs})
	for i := 0; i < 3; i++ {
		l.get(t, remote.BlobPath(uRun, digestOf(l.app)))
	}
	if n := cs.collects.Load(); n != 3 {
		t.Fatalf("no cache: %d collects, want 3", n)
	}
}
