package remote

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func coordinatorFixture(t *testing.T) (*Lease, *store.Store, *lease.Engine) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for _, err := range []error{
		s.CreatePipeline(ctx, store.PipelineRow{ID: "p", Project: "proj", Revision: "rev", SourceTrust: "owner", Status: "queued", Trigger: "local", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full"}),
		s.CreateJob(ctx, store.JobRow{ID: "j", PipelineID: "p", Name: "build", Kind: "test", Idempotent: true, InputDigest: "i"}),
		s.CreateAttempt(ctx, store.AttemptRow{ID: "a", JobID: "j", N: 1}, ""),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	e := lease.New(s, nil, lease.Config{})
	epoch, err := e.Grant(ctx, "host", store.Demand{AttemptID: "a", LeaseID: "l", RunnerID: "node", CPU: 1, MemMB: 1, CapacityCPU: 2, CapacityMemMB: 2, TTL: time.Minute}, 1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := cas.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := entries.Facts{Project: "proj", Trust: "owner", Protection: "protected"}
	ns, err := entries.NamespaceOf(f)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{Ledger: s, Entries: entries.New(s, b, entries.Options{DisableFloor: true}), Blobs: b}
	l, err := h.Bind(Binding{AttemptID: "a", LeaseID: "l", Epoch: epoch, Facts: f,
		Plan: &protocol.LeaseCache{Namespace: base64.RawURLEncoding.EncodeToString(ns.Canonical()), Access: "rw", Save: []protocol.CacheSave{{Spec: "go", Key: strings.Repeat("a", 64), Dirs: []string{"go/mod"}}}},
		Fence: func(ctx context.Context) error {
			if err := e.Fence(ctx, "l", epoch); err != nil {
				return errors.Join(transfer.ErrStale, err)
			}
			return nil
		},
		Ready: func(ctx context.Context) error {
			state, n, err := s.AttemptStateEpoch(ctx, "a")
			if err == nil && state == "leased" {
				return s.Transition(ctx, "a", "leased", "running", store.TransitionDetail{Epoch: n})
			}
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return l, s, e
}

func assertAudit(t *testing.T, s *store.Store, reason string) {
	t.Helper()
	rows, err := s.AuditEntries(context.Background(), 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Action == "cache.remote.refused" && r.Detail == "reason="+reason {
			return
		}
	}
	t.Fatalf("no %s refusal audit", reason)
}

func TestCacheRefOutsidePlanAudited(t *testing.T) {
	l, s, _ := coordinatorFixture(t)
	err := l.CacheRef(context.Background(), protocol.CacheRef{LeaseID: "l", Spec: "go", Key: strings.Repeat("b", 64)})
	var refused *entries.Refused
	if !errors.As(err, &refused) || refused.Reason != entries.ReasonNotPlanned {
		t.Fatalf("refusal=%v", err)
	}
	assertAudit(t, s, entries.ReasonNotPlanned)
}

func TestCacheRefReadOnlyAudited(t *testing.T) {
	l, s, _ := coordinatorFixture(t)
	l.b.Plan.Access = "ro"
	err := l.CacheRef(context.Background(), protocol.CacheRef{LeaseID: "l", Spec: "go", Key: strings.Repeat("a", 64)})
	var refused *entries.Refused
	if !errors.As(err, &refused) || refused.Reason != entries.ReasonNotPlanned {
		t.Fatalf("refusal=%v", err)
	}
	assertAudit(t, s, entries.ReasonNotPlanned)
}

func TestCacheFinalizeMissingRefsBestEffort(t *testing.T) {
	l, s, e := coordinatorFixture(t)
	ctx := context.Background()
	if err := l.Declare(ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.Result(ctx, "l", l.b.Epoch); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := l.Finalize(ctx, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("missing refs exceeded bounded wait")
	}
	rows, err := s.CacheEntries(ctx, "")
	if err != nil || len(rows) != 0 {
		t.Fatalf("entries=%v err=%v", rows, err)
	}
}

func TestCacheFinalizeDoesNotOverrideFailure(t *testing.T) {
	l, s, e := coordinatorFixture(t)
	ctx := context.Background()
	if err := l.Declare(ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.Result(ctx, "l", l.b.Epoch); err != nil {
		t.Fatal(err)
	}
	if err := l.h.Entries.RecordResult(ctx, "a", l.b.Epoch, false); err != nil {
		t.Fatal(err)
	}
	if err := l.Finalize(ctx, time.Millisecond); err == nil {
		t.Fatal("finalizer overwrote recorded failure")
	}
	rows, err := s.CacheEntries(ctx, "")
	if err != nil || len(rows) != 0 {
		t.Fatalf("entries=%v err=%v", rows, err)
	}
}

func TestCacheFinalizeLateRefAndRetract(t *testing.T) {
	l, s, e := coordinatorFixture(t)
	ctx := context.Background()
	if err := l.Declare(ctx); err != nil {
		t.Fatal(err)
	}
	digest, n, err := l.h.Blobs.Put(ctx, cacheRealm(l.b.Plan.Namespace), bytes.NewReader(cacheArchive(t, "payload", false)))
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Result(ctx, "l", l.b.Epoch); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- l.Finalize(ctx, time.Second) }()
	if err = l.CacheRef(ctx, protocol.CacheRef{LeaseID: "l", Spec: "go", Key: strings.Repeat("a", 64), Blobs: []protocol.CacheBlob{{Path: "mod", SHA256: digest, Size: n}}}); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	rows, err := s.CacheEntries(ctx, "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("entries=%v err=%v", rows, err)
	}
	if rows[0].Writer.Epoch != l.b.Epoch {
		t.Fatal("writer epoch not fenced")
	}
	if err = l.Retract(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err = s.CacheEntries(ctx, "")
	if err != nil || len(rows) != 0 {
		t.Fatalf("retracted entries=%v err=%v", rows, err)
	}
}

func TestCacheReceiverLeaseBudget(t *testing.T) {
	l, s, _ := coordinatorFixture(t)
	l.h.Entries = entries.New(s, l.h.Blobs, entries.Options{DisableFloor: true, Getenv: func(name string) string {
		if name == "NSELF_CI_CACHE_MAX_BYTES" {
			return "7"
		}
		return ""
	}})
	var err error
	l, err = l.h.Bind(l.b)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	hdr := func(data string) transport.BlobHeader {
		return transport.BlobHeader{Stream: StreamCache, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(data))), Size: int64(len(data))}
	}
	w, err := l.Blob(ctx, hdr("data"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err = l.Blob(ctx, hdr("more")); !transfer.IsReason(err, transfer.ReasonSizeCap) {
		t.Fatalf("budget refusal=%v", err)
	}
}

func TestCacheNilPlanRefused(t *testing.T) {
	l, _, _ := coordinatorFixture(t)
	l.b.Plan = nil
	var err error
	l, err = l.h.Bind(l.b)
	if err != nil {
		t.Fatal(err)
	}
	err = l.CacheRef(context.Background(), protocol.CacheRef{})
	var refusal *entries.Refused
	if !errors.As(err, &refusal) || refusal.Reason != entries.ReasonNotPlanned {
		t.Fatalf("nil plan refusal=%v", err)
	}
}
