package roles

// Purpose: audit rows for what the decision path never sees (D-1276): a refused sign-in (a certificate or a
// token that does not resolve to an actor) and a refused admin mutation (a validation or role refusal).
// Inputs:  who tried, how, and the E687 error; the peer address when the transport gave one (WithPeer).
// Outputs: a "signin.fail" row per bucket and window, and a "<action>.refused" row per refused mutation.
// Constraints: a sign-in failure row is rate limited, so a guessing run cannot flood the log: one row per
//          peer and kind per window, one more row per window of the whole log's failures once a global
//          budget is spent; the rows that were held back are counted in the next row (suppressed=N). A
//          row names the reason key, the peer and the principal or token id when one is known, never the
//          token text. Writing is best effort: the sign-in was refused already, so a failed append does
//          not change the answer. Rows for a store failure (no reason key) are not written.
//
// Audit rows of a remote mutation (D-1276, decision): a mutation that calls Authorize and then writes its
// own row leaves two rows on purpose. The authorize row is the decision (allow or deny, with the role and
// project asked for) and exists whether or not the mutation follows; the mutation row is written first and
// fails closed, so no action happens without it. Both carry the same actor, and the target of the
// mutation row names the object. One combined row would drop the denial of a request that never reaches
// its mutation. Member-level decisions write no authorize row, so those mutations leave one row.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
)

const (
	signInWindow = 30 * time.Second
	signInBudget = 20   // rows per window across every peer
	signInTrack  = 4096 // peers remembered before the table is cleared
)

type signInBucket struct {
	start      time.Time
	suppressed int
}

// signInLimiter is the table behind the rate limit; the zero value is ready.
type signInLimiter struct {
	mu         sync.Mutex
	by         map[string]*signInBucket
	globalAt   time.Time
	globalUsed int
	globalHeld int
}

// allow reports whether a row may be written for key now, and how many rows were held back since the
// last one that was written for it.
func (l *signInLimiter) allow(key string, now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.by == nil || len(l.by) >= signInTrack {
		l.by = map[string]*signInBucket{}
	}
	b := l.by[key]
	if b != nil && now.Sub(b.start) < signInWindow {
		b.suppressed++
		return false, 0
	}
	if now.Sub(l.globalAt) >= signInWindow {
		l.globalAt, l.globalUsed = now, 0
	}
	if l.globalUsed >= signInBudget {
		if b != nil {
			b.suppressed++
		}
		l.globalHeld++
		return false, 0
	}
	l.globalUsed++
	held := l.globalHeld
	l.globalHeld = 0
	if b != nil {
		held += b.suppressed
	}
	l.by[key] = &signInBucket{start: now}
	return true, held
}

type peerKey struct{}

// WithPeer records the remote address of a request, so a sign-in failure row names it and the rate limit
// is per peer.
func WithPeer(ctx context.Context, addr string) context.Context {
	return context.WithValue(ctx, peerKey{}, addr)
}

// auditSignInFailure writes the rate-limited row of a refused sign-in. kind is "token" or "certificate".
func (s *Service) auditSignInFailure(ctx context.Context, who, kind string, err error) {
	reason := ReasonOf(err)
	if s.Log == nil || reason == "" {
		return
	}
	peer, _ := ctx.Value(peerKey{}).(string)
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	ok, held := s.lim.allow(kind+"|"+peer, now)
	if !ok {
		return
	}
	detail := "reason=" + reason
	if peer != "" {
		detail += fmt.Sprintf(" peer=%q", peer)
	}
	if held > 0 {
		detail += fmt.Sprintf(" suppressed=%d", held)
	}
	_ = s.Log.Append(ctx, audit.Event{Actor: who, Action: "signin.fail", Target: kind, Detail: detail})
}

// refused writes the row of a refused mutation and returns err unchanged. Only a refusal with a reason
// key is a row: a store failure is not a decision.
func (s *Service) refused(ctx context.Context, by Actor, action, target string, err error) error {
	reason := ReasonOf(err)
	if err == nil || s.Log == nil || reason == "" {
		return err
	}
	who := "unknown"
	if by.id != "" && by.kind != "" {
		who = by.Label()
	}
	if r := []rune(target); len(r) > 64 {
		target = string(r[:64])
	}
	target = strings.ReplaceAll(fmt.Sprintf("%q", target), " ", "_")
	_ = s.Log.Append(ctx, audit.Event{Actor: who, Action: action + ".refused", Target: target, Detail: "reason=" + reason})
	return err
}
