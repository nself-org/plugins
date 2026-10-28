//go:build security

package redteam

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

// TM-38: read/check/reserve must share the writer transaction. Attackers
// start together and try to spend the same last unit of budget.
func TestC38ConcurrentReservations(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	start := make(chan struct{})
	var wg sync.WaitGroup
	var admitted atomic.Int64
	denied := errors.New("budget exhausted")
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := st.LedgerWrite(ctx, func(tx store.LedgerTx) error {
				held, err := tx.Reserved(store.ScopeDay, "", "2026-10-28")
				if err != nil {
					return err
				}
				if held >= 1 {
					return denied
				}
				return tx.Reserve(store.Reservation{Owner: fmt.Sprint(i), Scope: store.ScopeDay, WindowKey: "2026-10-28", Micro: 1})
			})
			if err == nil {
				admitted.Add(1)
			} else if !errors.Is(err, denied) {
				t.Errorf("reserve: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	held, err := st.Reserved(ctx, store.ScopeDay, "", "2026-10-28")
	if err != nil || held != 1 || admitted.Load() != 1 {
		t.Fatalf("held=%d admitted=%d err=%v", held, admitted.Load(), err)
	}
}

func TestC38NegativeAndOverflowingReservations(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	for _, n := range []int64{-1, math.MinInt64} {
		err := st.LedgerWrite(ctx, func(tx store.LedgerTx) error {
			return tx.Reserve(store.Reservation{Owner: "negative", Scope: store.ScopeDay, Micro: n})
		})
		if err == nil {
			t.Errorf("accepted negative reservation %d", n)
		}
	}
	for _, owner := range []string{"a", "b"} {
		if err := st.LedgerWrite(ctx, func(tx store.LedgerTx) error {
			return tx.Reserve(store.Reservation{Owner: owner, Scope: store.ScopeMonth, Micro: math.MaxInt64})
		}); err != nil {
			t.Fatal(err)
		}
	}
	if held, err := st.Reserved(ctx, store.ScopeMonth, "", ""); err != nil || held != math.MaxInt64 {
		t.Fatalf("overflow held=%d err=%v", held, err)
	}
}

// TM-32: a delivery labels a fork-only SHA and its tag as authoritative,
// then the forge refuses to corroborate it. The delivery cannot grant owner.
func TestC32ForkOnlyTagFailedRefetch(t *testing.T) {
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	factBool := func(v bool) model.Fact[bool] { return model.Fact[bool]{Value: &v, Source: "api"} }
	factString := func(v string) model.Fact[string] { return model.Fact[string]{Value: &v, Source: "api"} }
	upstream, fork := t.TempDir(), t.TempDir()
	for _, dir := range []string{upstream, fork} {
		aGit(t, dir, "init", "-q")
		aGit(t, dir, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-q", "-m", dir)
	}
	sha = aGit(t, fork, "rev-parse", "HEAD")
	aGit(t, fork, "tag", "release")

	h := run.SourceTrust{Getenv: func(string) string { return "" }, Refetch: func(_ context.Context, f model.TriggerFacts) (model.TriggerFacts, error) {
		if f.Revision != sha {
			t.Fatalf("refetch revision=%q", f.Revision)
		}
		// The SHA is real on the fork but cannot be read from upstream.
		if _, err := os.Stat(filepath.Join(upstream, ".git", "objects", sha[:2], sha[2:])); !os.IsNotExist(err) {
			t.Fatalf("fork SHA unexpectedly on upstream: %v", err)
		}
		return model.TriggerFacts{}, errors.New("tag SHA absent from upstream")
	}}
	f := model.TriggerFacts{Kind: "tag", Auth: "per-repo", Repo: "owner/upstream", Ref: "refs/tags/release", Revision: sha,
		ActorPermission: factString("admin"), HeadRepo: factString("owner/upstream"), SHAReachable: factBool(true), FirstTime: factBool(false)}
	if control := source.Classify(f); control.Class != "owner" {
		t.Fatalf("forged facts must exercise owner path: %+v", control)
	}

	res, err := h.ClassifyForge(context.Background(), t.TempDir(), f)
	if err != nil || res.Class != "untrusted" || !res.RequiresApproval {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	found := false
	for _, r := range res.Reasons {
		if r == source.ReasonRefetchFailed {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing refetch refusal: %+v", res)
	}
}
