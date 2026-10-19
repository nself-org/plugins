package runtime

// Purpose: tests of the handle bookkeeping of Driver.Run (CR findings of P7-HOST-16): a retried
// attempt dispatches a fresh run, a dispatched run is never left unreachable, two carriers of one
// lease share one run, an older carrier is fenced out.
// Inputs:  a stub Dispatcher that records its calls, memory books and a failing book.
// Outputs: pass or fail.
// Constraints: no network; the stub is used directly, not through the provider registry.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// stubDisp hands out run1, run2, ... and records what it was asked to do.
type stubDisp struct {
	mu        sync.Mutex
	dispatch  int
	cancelled []string
	onDisp    func()
}

func (s *stubDisp) Dispatch(context.Context, providers.Attempt) (providers.Handle, error) {
	s.mu.Lock()
	s.dispatch++
	n, hook := s.dispatch, s.onDisp
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	return providers.Handle{Provider: "stub", RunID: fmt.Sprintf("run%d", n), Nonce: fmt.Sprintf("n%d", n)}, nil
}
func (s *stubDisp) Liveness(context.Context, providers.Handle) (providers.LiveState, error) {
	return providers.Running, nil
}
func (s *stubDisp) Cancel(_ context.Context, h providers.Handle) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelled = append(s.cancelled, h.RunID)
	return nil
}
func (*stubDisp) Collect(context.Context, providers.Handle) (providers.Collected, error) {
	return providers.Collected{}, nil
}
func (*stubDisp) Profile() providers.LeaseProfile { return providers.LeaseProfile{} }
func (s *stubDisp) cancels() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.cancelled, ",")
}

func stubFacts(context.Context, model.Capability, protocol.Lease) (AttemptFacts, error) {
	return AttemptFacts{N: 1, Project: "o", Repo: "o/r"}, nil
}

// failingBook cannot store a handle.
type failingBook struct{ Book }

func (failingBook) Put(context.Context, string, Entry) (Entry, error) {
	return Entry{}, errors.New("disk full")
}

// rivalBook says another carrier already stored run "rival" for the lease.
type rivalBook struct{ Book }

func (rivalBook) Put(_ context.Context, _ string, e Entry) (Entry, error) {
	return Entry{Handle: providers.Handle{Provider: "stub", RunID: "rival", Nonce: "nr"}, Epoch: e.Epoch}, nil
}

func attachAt(ctx context.Context, book Book, disp providers.Dispatcher, epoch uint64) (providers.Handle, error) {
	node, l := hostedLease("a1")
	l.Epoch = epoch
	return attach(ctx, book, disp, "stub", node, l, stubFacts)
}

func TestAttachReusesTheRunOfTheSameLeaseEpoch(t *testing.T) {
	ctx, book, d := context.Background(), NewMemBook(), &stubDisp{}
	first, err := attachAt(ctx, book, d, 7)
	if err != nil {
		t.Fatal(err)
	}
	again, err := attachAt(ctx, book, d, 7)
	if err != nil || again != first || d.dispatch != 1 {
		t.Fatalf("a second carrier of one lease must reuse the run: %+v vs %+v, dispatches %d, %v", again, first, d.dispatch, err)
	}
}

func TestAttachRetryAtAHigherEpochCancelsTheOldRunAndDispatchesAFreshOne(t *testing.T) {
	ctx, book, d := context.Background(), NewMemBook(), &stubDisp{}
	first, _ := attachAt(ctx, book, d, 7)
	second, err := attachAt(ctx, book, d, 8)
	if err != nil || second.RunID == first.RunID || d.dispatch != 2 || d.cancels() != first.RunID {
		t.Fatalf("the retry must start run 2 and stop run 1: first=%+v second=%+v dispatches=%d cancelled=%q err=%v", first, second, d.dispatch, d.cancels(), err)
	}
	if e, ok, _ := book.Get(ctx, "a1"); !ok || e.Handle.RunID != second.RunID || e.Epoch != 8 {
		t.Fatalf("the book must hold the retry's run: %+v %v", e, ok)
	}
}

func TestAttachOlderCarrierIsFencedOut(t *testing.T) {
	ctx, book, d := context.Background(), NewMemBook(), &stubDisp{}
	if _, err := attachAt(ctx, book, d, 9); err != nil {
		t.Fatal(err)
	}
	if _, err := attachAt(ctx, book, d, 8); codeOf(err) != "E708" || d.dispatch != 1 || d.cancels() != "" {
		t.Fatalf("a carrier of a superseded lease must be refused and must not touch the new run: %v dispatches=%d cancelled=%q", err, d.dispatch, d.cancels())
	}
}

// A handle that cannot be stored would leave a live run nobody can cancel: it is cancelled at once.
func TestAttachCancelsTheRunWhenTheHandleCannotBeKept(t *testing.T) {
	d := &stubDisp{}
	if _, err := attachAt(context.Background(), failingBook{NewMemBook()}, d, 7); err == nil || d.cancels() != "run1" {
		t.Fatalf("err=%v cancelled=%q, want the dispatched run cancelled", err, d.cancels())
	}
}

// Two carriers of one lease: the loser stops its own run and polls the winner's.
func TestAttachLoserCancelsItsOwnRunAndFollowsTheWinner(t *testing.T) {
	d := &stubDisp{}
	h, err := attachAt(context.Background(), rivalBook{NewMemBook()}, d, 7)
	if err != nil || h.RunID != "rival" || d.cancels() != "run1" {
		t.Fatalf("handle=%+v cancelled=%q err=%v", h, d.cancels(), err)
	}
}

// A lease cancelled right after Dispatch still gets its handle recorded, so the run can be cancelled.
func TestAttachRecordsTheHandleEvenWhenTheLeaseIsCancelledMidDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	book, d := NewMemBook(), &stubDisp{onDisp: func() {}}
	d.onDisp = cancel
	h, err := attachAt(ctx, book, d, 7)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok, _ := book.Get(context.Background(), "a1"); !ok || e.Handle != h {
		t.Fatalf("the handle must be in the book: %+v %v", e, ok)
	}
}

// TestDriverRetryAfterVanishDispatchesAgain: a vanished run forgets its handle, so the retry of the lost
// attempt (higher epoch) starts a second run instead of polling the dead one.
func TestDriverRetryAfterVanishDispatchesAgain(t *testing.T) {
	p := useProvider(t)
	node, l := hostedLease("r1")
	sink, clk := &recSink{}, newClock()
	defaultDriver.SetClock(clk.Sleep, clk.Now)
	p.live = providers.Vanished
	if err := defaultDriver.Run(context.Background(), node, l, sink); codeOf(err) != "E704" {
		t.Fatalf("vanish: %v", err)
	}
	if _, ok, _ := defaultDriver.Book().Get(context.Background(), "r1"); ok {
		t.Fatal("a vanished run's handle must be forgotten")
	}
	p.live, l.Epoch = providers.Finished, l.Epoch+1
	if err := defaultDriver.Run(context.Background(), node, l, sink); err != nil {
		t.Fatal(err)
	}
	if n, _ := p.counts(); n != 2 || len(sink.got()) != 1 {
		t.Fatalf("dispatches=%d results=%d, want 2 and 1", n, len(sink.got()))
	}
}

func TestDriverIsDormantWithoutV15(t *testing.T) {
	useProvider(t)
	t.Setenv("NSELF_V15", "")
	node, l := hostedLease("d1")
	if err := defaultDriver.Run(context.Background(), node, l, &recSink{}); codeOf(err) != "E708" {
		t.Fatalf("err=%v", err)
	}
}

func TestDefaultFactsSourceFailsClosed(t *testing.T) {
	useProvider(t)
	defaultDriver.SetFacts(nil)
	node, l := hostedLease("d2")
	if err := defaultDriver.Run(context.Background(), node, l, &recSink{}); codeOf(err) != "E708" {
		t.Fatalf("with no facts source a hosted run must be refused, got %v", err)
	}
	if n, _ := prov.counts(); n != 0 {
		t.Fatal("dispatched without a facts source")
	}
}

// HasSecrets must be false when any source does not vouch for itself: the coordinator applies it to
// every request the one ingress handles.
func TestIngressHasSecretsNeedsEverySource(t *testing.T) {
	useProvider(t)
	if (IngressAdapter{}).HasSecrets() {
		t.Fatal("the fake source holds no secrets of its own, so the adapter must not claim any")
	}
}

var credCalls struct {
	sync.Mutex
	n int
}

func TestContributorCachesTheCredentialCheck(t *testing.T) {
	useProvider(t)
	if err := RegisterCredential("cred-count", func(context.Context) rest.AuthFunc {
		credCalls.Lock()
		credCalls.n++
		credCalls.Unlock()
		return func(context.Context) (string, error) {
			return "", providers.NewError(providers.Config, "E700", "none", 0, time.Time{})
		}
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := defaultContributor.credential(context.Background(), "cred-count"); codeOf(err) != "E700" {
			t.Fatalf("err=%v", err)
		}
	}
	credCalls.Lock()
	defer credCalls.Unlock()
	if credCalls.n != 1 {
		t.Fatalf("the check ran %d times within the TTL, want once", credCalls.n)
	}
}

// An older carrier whose Dispatch was slow loses the Put race to the retry: it stops its own run and
// is fenced out, it must not adopt, cancel or delete the retry's run.
func TestAttachOlderCarrierLosingTheRaceToARetryIsFenced(t *testing.T) {
	ctx, book := context.Background(), NewMemBook()
	d := &stubDisp{}
	d.onDisp = func() {
		if _, err := book.Put(ctx, "a1", Entry{Handle: providers.Handle{Provider: "stub", RunID: "retry", Nonce: "nr"}, Epoch: 8}); err != nil {
			t.Error(err)
		}
	}
	if _, err := attachAt(ctx, book, d, 7); codeOf(err) != "E708" {
		t.Fatalf("err=%v, want E708", err)
	}
	if d.cancels() != "run1" {
		t.Fatalf("cancelled %q, want only the loser's own run1", d.cancels())
	}
	if e, ok, _ := book.Get(ctx, "a1"); !ok || e.Handle.RunID != "retry" {
		t.Fatalf("the retry's entry must be untouched: %+v %v", e, ok)
	}
}

// failingSink cannot take the result.
type failingSink struct{ *recSink }

func (failingSink) Result(context.Context, protocol.Result) error { return errors.New("store down") }

// A run that finished and was collected keeps its handle when only the delivery of the result fails, so a
// second carrier of the same lease collects again instead of paying for another run.
func TestDriverKeepsTheHandleWhenOnlyTheResultDeliveryFails(t *testing.T) {
	p := useProvider(t)
	node, l := hostedLease("k1")
	if err := defaultDriver.Run(context.Background(), node, l, failingSink{&recSink{}}); err == nil {
		t.Fatal("the sink failure must surface")
	}
	if _, ok, _ := defaultDriver.Book().Get(context.Background(), "k1"); !ok {
		t.Fatal("the handle of a finished run must survive a failed delivery")
	}
	sink := &recSink{}
	if err := defaultDriver.Run(context.Background(), node, l, sink); err != nil {
		t.Fatal(err)
	}
	if n, _ := p.counts(); n != 1 || len(sink.got()) != 1 {
		t.Fatalf("dispatches=%d results=%d, want 1 and 1", n, len(sink.got()))
	}
}

func TestRegisterCredentialRefusesBuiltInProviders(t *testing.T) {
	for _, id := range []string{"github", "gitlab", ""} {
		if err := RegisterCredential(id, func(context.Context) rest.AuthFunc { return nil }); codeOf(err) != "E700" {
			t.Errorf("%q: %v", id, err)
		}
	}
}
