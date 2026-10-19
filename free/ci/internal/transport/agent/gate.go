package agent

// Purpose: the job-token gate (Epic D5, R-DS-15): every lease-scoped call of a pull agent presents the token of
//          its lease; the gate verifies it, checks it against the lease, and binds it on first use to the
//          certificate serial and the session that used it.
// Inputs:  an agentapi.Call (node, session, certificate serial, lease, epoch, token, reconcile flag); the token
//          verifier (pki.Tokens), the durable bindings and the lease view (store.Store).
// Outputs: nil to admit; a *Denial (E661 + reason key) to refuse; any other error means the coordinator failed.
// Constraints: checks run in a fixed order (verify, runner, job, terminal, binding) so each refusal has one
//          reason; a binding is durable (the store), so a coordinator restart does not forget it; another
//          certificate is always refused; another session of the same certificate is refused unless the
//          agent says it kept this very lease across a reconnect (the first heartbeat of its session), in
//          which case the binding moves to the new session by compare and swap; a token whose job result was
//          accepted is spent; no token or key ever reaches a message or a log.

import (
	"context"
	"errors"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Verifier checks a token (pki.Tokens).
type Verifier interface {
	Verify(token string) (pki.TokenClaims, error)
}

// Bindings is the durable side of the gate (store.Store).
type Bindings interface {
	TokenBind(ctx context.Context, b store.TokenBinding) (store.TokenBinding, bool, error)
	TokenRebind(ctx context.Context, jti, serial, from, to string) (bool, error)
	TokenSpend(ctx context.Context, jti string) error
	TokenCheck(ctx context.Context, jti string) (store.TokenBinding, bool, error)
}

// LeaseView reads the lease a call names (store.Store).
type LeaseView interface {
	LeaseGet(ctx context.Context, id string) (store.RemoteLease, error)
}

// Gate admits or refuses lease-scoped calls.
type Gate struct {
	Tokens   Verifier
	Bindings Bindings
	Leases   LeaseView

	mu    sync.Mutex
	known map[use]bool // bindings this process already proved, so a log line costs no write
}

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
	cl, err := g.Tokens.Verify(c.Token)
	if err != nil {
		return deny(tokenReason(err), "lease %s: the job token is not valid", c.LeaseID)
	}
	if cl.RunnerID != c.Node {
		return deny(ReasonWrongRunner, "lease %s: the token was issued to another node", c.LeaseID)
	}
	l, err := g.Leases.LeaseGet(ctx, c.LeaseID)
	switch {
	case isStale(err):
		return deny(ReasonTerminal, "lease %s: the attempt is over", c.LeaseID)
	case err != nil:
		return err
	}
	if cl.AttemptID != l.AttemptID || cl.Epoch != int64(c.Epoch) {
		return deny(ReasonWrongJob, "lease %s: the token was issued for another job or attempt", c.LeaseID)
	}
	if terminal[l.State] {
		return deny(ReasonTerminal, "lease %s: the attempt is over", c.LeaseID)
	}
	return g.bind(ctx, c, cl)
}

func isStale(err error) bool {
	var se *store.Error
	return errors.As(err, &se) && se.Code == "E662"
}

func (g *Gate) bind(ctx context.Context, c agentapi.Call, cl pki.TokenClaims) error {
	u := use{cl.JTI, c.Serial, c.Session}
	g.mu.Lock()
	hit := g.known[u]
	g.mu.Unlock()
	if hit {
		return nil
	}
	if err := g.bindOnce(ctx, c, cl); err != nil {
		return err
	}
	g.mu.Lock()
	if g.known == nil || len(g.known) >= maxKnown {
		g.known = map[use]bool{}
	}
	g.known[u] = true
	g.mu.Unlock()
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
	case b.Spent:
		return deny(ReasonReplayed, "lease %s: the result for this token was already accepted", c.LeaseID)
	case b.Serial != c.Serial:
		return deny(ReasonWrongCertificate, "lease %s: the token is bound to another certificate", c.LeaseID)
	case b.Session == c.Session:
		return nil
	case !c.Reconcile:
		return deny(ReasonWrongSession, "lease %s: the token is bound to another session", c.LeaseID)
	}
	moved, err := g.Bindings.TokenRebind(ctx, cl.JTI, c.Serial, b.Session, c.Session)
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
	if !found || now.Serial != c.Serial || now.Session != c.Session {
		return deny(ReasonWrongSession, "lease %s: the token is bound to another session", c.LeaseID)
	}
	return nil
}

// forgetOtherSessions drops the proved bindings of jti that belong to a session other than keep.
func (g *Gate) forgetOtherSessions(jti, keep string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for u := range g.known {
		if u.jti == jti && u.session != keep {
			delete(g.known, u)
		}
	}
}

// Spend marks the token of a finished job used up; call it after the result was accepted.
func (g *Gate) Spend(ctx context.Context, jti string) error {
	g.mu.Lock()
	for u := range g.known {
		if u.jti == jti {
			delete(g.known, u)
		}
	}
	g.mu.Unlock()
	return g.Bindings.TokenSpend(ctx, jti)
}
