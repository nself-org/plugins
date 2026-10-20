package agent

// Purpose: the job-token gate (Epic D5, R-DS-15): every lease-scoped call of a pull agent presents the token of
//          its lease; the gate verifies it, checks it against the lease, and binds it on first use to the
//          certificate serial and the session that used it.
// Inputs:  an agentapi.Call (node, session, certificate serial, lease, epoch, token, reconcile flag); the token
//          verifier (pki.Tokens), the durable bindings and the lease view (store.Store).
// Outputs: nil to admit; a *Denial (E661 + reason key) to refuse; any other error means the coordinator failed.
//          Cost: a token's signature is verified once per process (the claims are cached until the token expires),
//          and the lease is read on a heartbeat or result frame and on the first frame of a token, not on
//          every log, artifact-ref or cache-ref frame; a lease found over drops the cache so the next frame reads it again.
// Constraints: checks run in a fixed order (verify, runner, job, terminal, binding) so each refusal has one
//          reason; a binding is durable (the store), so a coordinator restart does not forget it; another
//          certificate is always refused; another session of the same certificate is refused unless the
//          agent says it kept this very lease across a reconnect (the first heartbeat of its session), in
//          which case the binding moves to the new session by compare and swap; a token whose job result was
//          accepted is spent; no token or key ever reaches a message or a log.

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// TokenVerifier checks a token (pki.Tokens).
type TokenVerifier interface {
	Verify(token string) (pki.TokenClaims, error)
}

// Bindings is the durable side of the gate (store.Store).
type Bindings interface {
	TokenBind(ctx context.Context, b store.TokenBinding) (store.TokenBinding, bool, error)
	TokenRebind(ctx context.Context, jti, fromSerial, fromSession, toSerial, toSession string) (bool, error)
	TokenSpend(ctx context.Context, jti string) error
	TokenCheck(ctx context.Context, jti string) (store.TokenBinding, bool, error)
	TokenPrune(ctx context.Context, nowUnix int64) (int64, error)
}

// LeaseView reads the lease a call names (store.Store).
type LeaseView interface {
	LeaseGet(ctx context.Context, id string) (store.RemoteLease, error)
}

// Gate admits or refuses lease-scoped calls.
type Gate struct {
	Tokens   TokenVerifier
	Bindings Bindings
	Leases   LeaseView
	Now      func() time.Time // the clock the cached claims expire on; time.Now when nil. Use the token verifier's clock.

	mu    sync.Mutex
	known map[use]bool    // bindings this process already proved, so a log line costs no write
	moves *byte           // replaced by every rebind: a bind that overlapped one does not cache what it proved
	spent map[string]bool // tokens whose result was accepted, so a racing first bind cannot cache them

	claims map[[sha256.Size]byte]pki.TokenClaims // verified tokens (by digest, never the text), valid until their Exp
	fresh  map[leaseUse]bool                     // token+lease pairs whose lease was read and matched; log-type frames trust it
}

type leaseUse struct{ jti, lease string }

type use struct{ jti, serial, session string }

// maxKnown bounds the proved-binding cache.
const maxKnown = 4096

var terminal = map[model.JobState]bool{"passed": true, "failed": true, "cancelled": true, "lost": true, "retryable": true, "needs-operator": true}

func tokenReason(err error) string {
	if pki.ReasonOf(err) == pki.ReasonTokenExpired {
		return ReasonExpired
	}
	return ReasonInvalid
}

// Check vets one call. It is agentapi.Config.Admit.
func (g *Gate) Check(ctx context.Context, c agentapi.Call) error {
	if c.Token == "" {
		return deny(ReasonMissing, "lease %s: the call carries no job token", c.LeaseID)
	}
	cl, err := g.verify(c.Token)
	if err != nil {
		return deny(tokenReason(err), "lease %s: the job token is not valid", c.LeaseID)
	}
	if cl.RunnerID != c.Node {
		return deny(ReasonWrongRunner, "lease %s: the token was issued to another node", c.LeaseID)
	}
	if cl.Epoch == int64(c.Epoch) && !rereads(c.Type) && g.isFresh(cl.JTI, c.LeaseID) {
		return g.bind(ctx, c, cl)
	}
	l, err := g.Leases.LeaseGet(ctx, c.LeaseID)
	switch {
	case isStale(err):
		g.forgetLease(cl.JTI, c.LeaseID)
		return deny(ReasonTerminal, "lease %s: the attempt is over", c.LeaseID)
	case err != nil:
		return err
	}
	if cl.AttemptID != l.AttemptID || cl.Epoch != int64(c.Epoch) {
		return deny(ReasonWrongJob, "lease %s: the token was issued for another job or attempt", c.LeaseID)
	}
	if terminal[l.State] {
		g.forgetLease(cl.JTI, c.LeaseID)
		return deny(ReasonTerminal, "lease %s: the attempt is over", c.LeaseID)
	}
	if err := g.bind(ctx, c, cl); err != nil {
		return err
	}
	g.markFresh(cl.JTI, c.LeaseID)
	return nil
}

// rereads says whether a frame type always reads the lease: the state-changing ones do, the bulk ones trust the last read.
func rereads(typ string) bool { return typ == "heartbeat" || typ == "result" }

// verify is Tokens.Verify with a cache: a token is checked once, then trusted until its expiry.
func (g *Gate) verify(token string) (pki.TokenClaims, error) {
	key := sha256.Sum256([]byte(token))
	g.mu.Lock()
	cl, ok := g.claims[key]
	g.mu.Unlock()
	if ok && cl.Exp > g.now().Unix() {
		return cl, nil
	}
	cl, err := g.Tokens.Verify(token)
	g.mu.Lock()
	defer g.mu.Unlock()
	if err != nil {
		delete(g.claims, key)
		return cl, err
	}
	if g.claims == nil || len(g.claims) >= maxKnown {
		g.claims = map[[sha256.Size]byte]pki.TokenClaims{}
	}
	g.claims[key] = cl
	return cl, nil
}

func (g *Gate) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

func (g *Gate) isFresh(jti, lease string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.fresh[leaseUse{jti, lease}]
}

func (g *Gate) markFresh(jti, lease string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.spent[jti] { // spent while this call was checking: do not let log frames skip the lease for it
		return
	}
	if g.fresh == nil || len(g.fresh) >= maxKnown {
		g.fresh = map[leaseUse]bool{}
	}
	g.fresh[leaseUse{jti, lease}] = true
}

func (g *Gate) forgetLease(jti, lease string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.fresh, leaseUse{jti, lease})
}

func isStale(err error) bool {
	var se *store.Error
	return errors.As(err, &se) && se.Code == "E662"
}

func (g *Gate) bind(ctx context.Context, c agentapi.Call, cl pki.TokenClaims) error {
	u := use{cl.JTI, c.Serial, c.Session}
	g.mu.Lock()
	hit, spent, moves := g.known[u], g.spent[cl.JTI], g.moves
	g.mu.Unlock()
	if spent {
		return deny(ReasonReplayed, "lease %s: the result for this token was already accepted", c.LeaseID)
	}
	if hit {
		return nil
	}
	if err := g.bindOnce(ctx, c, cl); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.spent[cl.JTI] { // spent while this call was binding: do not remember it as usable
		return deny(ReasonReplayed, "lease %s: the result for this token was already accepted", c.LeaseID)
	}
	if g.moves != moves { // a rebind ran meanwhile: this session may no longer hold the token, so prove it again next time
		return nil
	}
	if g.known == nil || len(g.known) >= maxKnown {
		g.known = map[use]bool{}
	}
	g.known[u] = true
	return nil
}

func (g *Gate) bindOnce(ctx context.Context, c agentapi.Call, cl pki.TokenClaims) error {
	b, created, err := g.Bindings.TokenBind(ctx, store.TokenBinding{JTI: cl.JTI, Serial: c.Serial, Session: c.Session, NodeID: c.Node, Exp: cl.Exp})
	if err != nil {
		return err
	}
	switch {
	case created:
		return nil
	case b.Serial != c.Serial && !c.Reconcile:
		return deny(ReasonWrongCertificate, "lease %s: the token is bound to another certificate", c.LeaseID)
	case b.Spent:
		return deny(ReasonReplayed, "lease %s: the result for this token was already accepted", c.LeaseID)
	case b.Session == c.Session && b.Serial == c.Serial:
		return nil
	case !c.Reconcile:
		return deny(ReasonWrongSession, "lease %s: the token is bound to another session", c.LeaseID)
	}
	// Same node, new session: a reconnect, or after a certificate renewal a new serial (the listener authenticated it as this node).
	moved, err := g.Bindings.TokenRebind(ctx, cl.JTI, b.Serial, b.Session, c.Serial, c.Session)
	if err != nil {
		return err
	}
	if moved {
		g.forgetOtherSessions(cl.JTI, c.Session)
		return nil
	}
	// Lost a race: admitted only if the winner was this very session.
	now, found, err := g.Bindings.TokenCheck(ctx, cl.JTI)
	if err != nil {
		return err
	}
	if found && now.Spent { // the rebind may have failed because the result was accepted meanwhile
		return deny(ReasonReplayed, "lease %s: the result for this token was already accepted", c.LeaseID)
	}
	if !found || now.Serial != c.Serial || now.Session != c.Session {
		return deny(ReasonWrongSession, "lease %s: the token is bound to another session", c.LeaseID)
	}
	return nil
}

// forgetOtherSessions drops the proved bindings of jti that belong to a session other than keep.
func (g *Gate) forgetOtherSessions(jti, keep string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.moves = new(byte)
	for u := range g.known {
		if u.jti == jti && u.session != keep {
			delete(g.known, u)
		}
	}
}

// Spend marks the token of a finished job used up; call it after the result was accepted.
func (g *Gate) Spend(ctx context.Context, jti string) error {
	g.mu.Lock()
	if g.spent == nil || len(g.spent) >= maxKnown {
		g.spent = map[string]bool{}
	}
	g.spent[jti] = true
	for k := range g.fresh {
		if k.jti == jti {
			delete(g.fresh, k)
		}
	}
	for u := range g.known {
		if u.jti == jti {
			delete(g.known, u)
		}
	}
	g.mu.Unlock()
	return g.Bindings.TokenSpend(ctx, jti)
}

// Prune removes the bindings of tokens that expired more than an hour ago.
func (g *Gate) Prune(ctx context.Context, now time.Time) (int64, error) {
	return g.Bindings.TokenPrune(ctx, now.Unix())
}
