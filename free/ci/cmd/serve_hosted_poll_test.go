package main

// Purpose: prove the coordinator registers hosted profiles and selects only owned, current handles.
// Inputs: table-driven durable lease views and an in-memory dispatch book.
// Outputs: exact provider polling calls and propagated failures.
// Constraints: no network, credentials or provider dispatch.

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/fake"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/hosted"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestPollHostedLeases(t *testing.T) {
	boom := errors.New("provider unavailable")
	for _, tc := range []struct {
		name, owner, provider string
		epoch                 int64
		handle, want          bool
		pollErr               error
	}{
		{name: "owned hosted", owner: "ours", provider: "hosted", epoch: 7, handle: true, want: true},
		{name: "foreign coordinator", owner: "other", provider: "hosted", epoch: 7, handle: true},
		{name: "agent has no handle", owner: "ours"},
		{name: "stale dispatch fence", owner: "ours", provider: "hosted", epoch: 6, handle: true},
		{name: "empty provider", owner: "ours", epoch: 7, handle: true},
		{name: "provider error", owner: "ours", provider: "hosted", epoch: 7, handle: true, want: true, pollErr: boom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			book := runtime.NewMemBook()
			if tc.handle {
				if _, err := book.Put(ctx, "attempt", runtime.Entry{Handle: providers.Handle{Provider: tc.provider}, Epoch: tc.epoch}); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			poll := func(_ context.Context, provider, id, attempt string, epoch int64) (bool, lease.LiveState, error) {
				calls++
				if provider != tc.provider || id != "lease" || attempt != "attempt" || epoch != 7 {
					t.Fatalf("wrong poll: %s %s %s %d", provider, id, attempt, epoch)
				}
				return true, lease.LiveRunning, tc.pollErr
			}
			err := pollHostedLeases(ctx, []store.RemoteLease{{ID: "lease", AttemptID: "attempt", CoordinatorID: tc.owner, Epoch: 7}}, book,
				func(_ context.Context, id string) bool { return id == "ours" }, poll)
			if (calls == 1) != tc.want || calls > 1 || !errors.Is(err, tc.pollErr) {
				t.Fatalf("calls=%d error=%v, want poll=%v error=%v", calls, err, tc.want, tc.pollErr)
			}
		})
	}
}

type pollingProvider struct {
	*fake.Fake
	polled chan struct{}
}

func (*pollingProvider) ID() string { return "serve-polling-probe" }
func (p *pollingProvider) Liveness(context.Context, providers.Handle) (providers.LiveState, error) {
	select {
	case p.polled <- struct{}{}:
	default:
	}
	return providers.Running, nil
}

var pollingProbe = &pollingProvider{Fake: fake.New(fake.Options{}), polled: make(chan struct{}, 1)}
var pollingOnce sync.Once

// TestServeWirePollsHostedLease fails when the lifecycle hook is absent, even if the helper exists.
func TestServeWirePollsHostedLease(t *testing.T) {
	pollingOnce.Do(func() {
		if err := providers.Register(pollingProbe); err != nil {
			t.Fatal(err)
		}
	})
	select {
	case <-pollingProbe.polled:
	default:
	}
	ctx := context.Background()
	st := serveTestStore(t)
	reg, err := st.Register(ctx, os.Getpid(), "poll-test", "poll-test")
	if err != nil {
		t.Fatal(err)
	}
	seedPollingLease(t, st, reg.ID)
	stop, err := serveWire(ctx, st, t.TempDir(), reg.ID, &coordinator.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	select {
	case <-pollingProbe.polled:
	case <-time.After(5 * time.Second):
		t.Fatal("coordinator did not poll its active hosted lease")
	}
}

func seedPollingLease(t *testing.T, st *store.Store, coordinatorID string) {
	t.Helper()
	ctx := context.Background()
	seedAttempt(t, st, "owner")
	// The shared fixture's first attempt belongs to its implicit coordinator. A second job
	// supplies an explicitly process-owned attempt for the worker's ownership filter.
	if err := st.CreateJob(ctx, store.JobRow{ID: "poll-job", PipelineID: "pipe", Name: "poll", Kind: "test", Idempotent: true, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAttempt(ctx, store.AttemptRow{ID: "poll-att", JobID: "poll-job", CoordinatorID: coordinatorID, N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	eng := lease.New(st, nil, lease.Config{})
	epoch, err := eng.Grant(ctx, "host", store.Demand{AttemptID: "poll-att", LeaseID: "poll-lease", RunnerID: "hosted", CPU: 1, MemMB: 1, CapacityCPU: 1, CapacityMemMB: 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.StoreBook(st).Put(ctx, "poll-att", runtime.Entry{Handle: providers.Handle{Provider: pollingProbe.ID(), RunID: "run"}, Epoch: epoch}); err != nil {
		t.Fatal(err)
	}
}

func TestHostedPollingRegistersLinkedProvider(t *testing.T) {
	st := serveTestStore(t)
	eng := lease.New(st, nil, lease.Config{})
	stop, err := startHostedPolling(context.Background(), eng, runtime.StoreBook(st), func(context.Context, string) bool { return true }, nil)
	if err != nil {
		t.Fatal(err)
	}
	stop()
	// A second registration can fail only if coordinator startup installed the profile.
	if err := eng.RegisterProfile(hosted.ProviderID, lease.LeaseProfile{Liveness: func(context.Context, string) (lease.LiveState, error) {
		return lease.LiveRunning, nil
	}}); err == nil {
		t.Fatal("coordinator did not register the linked GitHub-hosted lease profile")
	}
}
