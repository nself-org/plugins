package pairing

// Purpose: the pairing service: offers, withdrawal and the facts the enrollment listener serves.
// Inputs:  Config (the CA, store, registry, a default Approver, the coordinator id, the agent URL).
// Outputs: Offer (code, root fingerprint, expiry), Withdraw, Hello.
// Constraints: a code lives 10 minutes and is shown once, to the caller of Offer; only its SHA-256 hash is
//          stored; each offer keeps its OWN Approver in memory (two offers never share a callback);
//          CertTTL 0 is 7 days and anything outside 1 minute..7 days is E660 cert_ttl; a short CertTTL
//          lineage is never renewed; every decision writes one audit row.

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// OfferTTL is how long a code lives.
const OfferTTL = 10 * time.Minute

// Alert describes a security event the operator must see (a reused code).
type Alert struct {
	Kind, OfferID, NodeID, Serial string
}

// Config configures a Service.
type Config struct {
	CA       *pki.CA
	Store    *store.Store
	Registry *registry.Registry
	// Approver is the default for offers that bring none (the interactive admin prompt).
	Approver Approver
	// CoordinatorID names the coordinator leaf (URI SAN nself-ci:coordinator/<id>).
	CoordinatorID string
	// AgentURL is returned to enrolled nodes (the agent API of the coordinator).
	AgentURL string
	// Actor is the audit actor of pairing decisions (default "coordinator").
	Actor string
	// AfterIssue runs after a certificate is issued (cmd wires lifecycle.RunHooks(pair) here).
	AfterIssue func(ctx context.Context, nodeID string) error
	// Alert receives security events; nil drops them (the audit row is written either way).
	Alert func(Alert)
	// Poll is the offer-watch interval of ServeWhileOffers (default 250 ms).
	Poll time.Duration
}

// Service owns the offers of one store.
type Service struct {
	cfg       Config
	mu        sync.Mutex
	approvers map[string]Approver
	redeemed  map[string]bool
	inflight  int
	proofs    proofAuditor
	wake      chan struct{}
}

// New returns a Service.
func New(cfg Config) *Service {
	if cfg.Actor == "" {
		cfg.Actor = "coordinator"
	}
	if cfg.Poll <= 0 {
		cfg.Poll = 250 * time.Millisecond
	}
	return &Service{cfg: cfg, approvers: map[string]Approver{}, redeemed: map[string]bool{}, wake: make(chan struct{}, 1)}
}

// OfferOptions describe one offer.
type OfferOptions struct {
	// Approver decides this offer; nil uses Config.Approver.
	Approver Approver
	// TTL is the code lifetime (0: 10 minutes; longer is refused).
	TTL   time.Duration
	Label string
	// CertTTL is the certificate lifetime (0: 7 days; outside 1 minute..7 days: E660 cert_ttl).
	CertTTL time.Duration
	// Intent binds the code to a burst intent; only intent-bound offers open the ServeWhileOffers listener.
	Intent string
}

// OfferResult is what the operator (or the scheduler) gets back.
type OfferResult struct {
	ID, Code, RootFingerprint string
	ExpiresAt                 time.Time
	CertTTL                   time.Duration
}

func clean(s string, max int) (string, bool) {
	if len(s) > max || strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "", false
	}
	return s, true
}

func (s *Service) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Offer creates a single-use code.
func (s *Service) Offer(ctx context.Context, o OfferOptions) (OfferResult, error) {
	ttl, err := pki.CheckTTL(o.CertTTL)
	if err != nil {
		return OfferResult{}, err
	}
	life := o.TTL
	if life == 0 {
		life = OfferTTL
	}
	label, ok1 := clean(o.Label, 64)
	intent, ok2 := clean(o.Intent, 128)
	if life < 0 || life > OfferTTL || !ok1 || !ok2 {
		return OfferResult{}, fail(ReasonBadRequest, "offer ttl, label or intent is invalid")
	}
	ap := o.Approver
	if ap == nil {
		ap = s.cfg.Approver
	}
	if ap == nil {
		return OfferResult{}, fail(ReasonNoApprover, "an offer needs an approver")
	}
	code, err := GenerateCode()
	if err != nil {
		return OfferResult{}, err
	}
	id, err := GenerateCode()
	if err != nil {
		return OfferResult{}, err
	}
	now := s.cfg.CA.Now()
	row := store.PKIOffer{ID: "offer-" + strings.ToLower(id), CodeHash: HashCode(code), Label: label, Intent: intent, CertTTL: int64(ttl), ExpiresAt: now.Add(life).UnixNano()}
	if err = s.cfg.Store.PKIOfferPut(ctx, row); err != nil {
		return OfferResult{}, err
	}
	s.mu.Lock()
	s.approvers[row.ID] = ap
	s.mu.Unlock()
	s.signal()
	return OfferResult{ID: row.ID, Code: code, RootFingerprint: s.cfg.CA.RootFingerprint(), ExpiresAt: now.Add(life), CertTTL: ttl}, nil
}

// Withdraw ends an offer and revokes the certificate already issued for it, if any.
func (s *Service) Withdraw(ctx context.Context, code string) error {
	o, err := s.cfg.Store.PKIOfferWithdraw(ctx, HashCode(code))
	if err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.approvers, o.ID)
	s.mu.Unlock()
	if o.Serial != "" {
		if _, err = s.cfg.Store.PKIDeny(ctx, s.cfg.Actor, o.NodeID, "offer_withdrawn", o.Serial); err != nil {
			return err
		}
	}
	// The audit row is written before the window is woken: waking it lets `nodes pair` exit and close the store, and an
	// append after that fails with "sql: database is closed".
	err = s.cfg.Store.AppendAudit(ctx, s.cfg.Actor, "pairing.withdraw", o.ID, "revoked="+boolStr(o.Serial != ""))
	s.signal()
	return err
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// Hello is the public reply to phase hello: the coordinator id and its clock.
func (s *Service) Hello() enroll.Hello {
	return enroll.Hello{Version: enroll.Version, CoordinatorID: s.cfg.CoordinatorID, ServerTime: s.cfg.CA.Now().Unix()}
}

func (s *Service) approverFor(offerID string) Approver {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.approvers[offerID]
}
