package cloudvm

// Purpose: enrollment of a burst VM (Epic D13, review F3/F4): one pairing offer per live intent, an
// Approver that approves only a node bound to that intent, and the withdrawal of the offer when the
// machine goes away. `ci serve` runs pairing.ServeWhileOffers, so the enrollment listener is open
// only while at least one intent-bound offer is live; this package makes the offers, it never listens.
// Inputs:  the intent token; at redemption the node name and CSR from pairing.
// Outputs: a pairing code for the user data, an approval or a denial (audited by pairing).
// Constraints: the offer lives 10 minutes (pairing.OfferTTL) and carries the intent; its certificate
// lives MaxLifetime + Grace; the Approver approves only when the intent is live and the node is named
// nselfci-<token> (the host name the VM gets from the server name), reports the node to the engine
// (NodeJoined) before the certificate is issued, and sets the trust fields from burst.trust; it
// never sees the code; no lease can precede redemption because the node does not exist before it.

import (
	"context"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Pairing is the part of pairing.Service the provider uses.
type Pairing interface {
	Offer(ctx context.Context, o pairing.OfferOptions) (pairing.OfferResult, error)
	Withdraw(ctx context.Context, code string) error
}

// Joiner is the part of burst.Engine the enrollment calls.
type Joiner interface {
	NodeJoined(ctx context.Context, token, nodeID string) error
}

// Intents says whether a burst intent of this coordinator is live.
type Intents interface {
	Live(ctx context.Context, token string) (bool, error)
}

// StoreIntents reads intents from the burst store.
type StoreIntents struct {
	Store       *store.Store
	Coordinator string
}

// Live implements Intents.
func (s StoreIntents) Live(ctx context.Context, token string) (bool, error) {
	in, ok, err := s.Store.BurstIntent(ctx, token)
	if err != nil {
		return false, err
	}
	return ok && in.Coordinator == s.Coordinator && in.State == store.BurstLive, nil
}

// NamePrefix starts the server name and the node name of every burst VM.
const NamePrefix = "nselfci-"

// NodeName is the server name, and so the host name and the node name, of an intent.
func NodeName(token string) string { return NamePrefix + strings.ToLower(token) }

// SetJoiner binds the engine that learns of a joined node (the engine and the provider need each other).
func (p *Provider) SetJoiner(j Joiner) {
	p.mu.Lock()
	p.joiner = j
	p.mu.Unlock()
}

func (p *Provider) joinerOf() Joiner {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.joiner
}

// certTTL is how long the node certificate lives: the longest the machine may.
func (p *Provider) certTTL() time.Duration {
	l := p.cfg.Lifetime
	if l.MaxLifetime <= 0 {
		l.MaxLifetime = burst.DefaultMaxLifetime
	}
	if l.Grace <= 0 {
		l.Grace = burst.DefaultGrace
	}
	return l.MaxLifetime + l.Grace
}

// offer makes the intent-bound pairing offer and remembers its code for withdrawal.
func (p *Provider) offer(ctx context.Context, token string) (pairing.OfferResult, error) {
	res, err := p.cfg.Pairing.Offer(ctx, pairing.OfferOptions{
		Approver: &approver{p: p, token: token}, TTL: pairing.OfferTTL, CertTTL: p.certTTL(), Intent: token, Label: "burst " + p.cfg.ProviderID,
	})
	if err != nil {
		return res, fail(ReasonEnroll, err, "the pairing offer for intent %s failed", token)
	}
	p.mu.Lock()
	p.offers[token] = res.Code
	p.mu.Unlock()
	return res, nil
}

// withdraw ends the offer of an intent (best effort: a missing offer is already gone).
func (p *Provider) withdraw(ctx context.Context, token string) {
	p.mu.Lock()
	code, ok := p.offers[token]
	delete(p.offers, token)
	p.mu.Unlock()
	if ok {
		_ = p.cfg.Pairing.Withdraw(context.WithoutCancel(ctx), code)
	}
}

// approver approves the one node an intent expects.
type approver struct {
	p     *Provider
	token string
}

// Kind implements pairing.Approver.
func (a *approver) Kind() string { return "policy:burst" }

// Approve implements pairing.Approver.
func (a *approver) Approve(ctx context.Context, r pairing.ApprovalRequest) (pairing.Decision, error) {
	deny := func(reason string) (pairing.Decision, error) { return pairing.Decision{Reason: reason}, nil }
	switch {
	case r.Intent != a.token:
		return deny("offer is not bound to this intent")
	case r.Existing:
		return deny("burst enrollment is for new nodes")
	case r.NodeID != NodeName(a.token):
		return deny("node name is not the one this intent expects")
	}
	live, err := a.p.cfg.Intents.Live(ctx, a.token)
	if err != nil {
		return pairing.Decision{}, err
	}
	if !live {
		return deny("intent is not live")
	}
	j := a.p.joinerOf()
	if j == nil {
		return deny("no burst engine is bound")
	}
	pol, err := a.p.cfg.Spend(ctx)
	if err != nil {
		return pairing.Decision{}, err
	}
	if err = j.NodeJoined(ctx, a.token, r.NodeID); err != nil {
		return deny("intent cannot take this node")
	}
	t := pol.Burst.Trust
	return pairing.Decision{Approve: true, Reason: "burst intent " + a.token + " isolation " + t.Isolation,
		Trust: pairing.TrustFields{Accepts: append([]string(nil), t.Accepts...), SecretClasses: append([]string(nil), t.Secrets...), PrivacyZone: t.PrivacyZone}}, nil
}

// tokenOfNode finds the intent token of a node named nselfci-<token> among the offers this process made.
func (p *Provider) tokenOfNode(nodeID string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for token := range p.offers {
		if NodeName(token) == nodeID {
			return token, true
		}
	}
	for _, token := range p.byID {
		if NodeName(token) == nodeID {
			return token, true
		}
	}
	return "", false
}
