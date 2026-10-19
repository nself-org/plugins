package pairing

// Purpose: redeeming a code (Epic D5): validate the request, consume the code, ask the offer's approver,
//          register the node, pin its key, issue the certificate, audit the decision.
// Inputs:  an enrollment Request from the listener and the source address.
// Outputs: an enroll.Response, or E659 (code) / E660 (clock, CSR, key pin) errors.
// Constraints: skew, request shape and CSR are checked BEFORE the code is consumed, so a broken or
//          skewed node does not burn the code; the code is consumed atomically (one winner); a second
//          presentation of a redeemed code revokes the certificate it produced, audits and alerts; a
//          denial or error after consumption spends the code (single use); the audit rows hold the CSR
//          fingerprint, approver kind, trust fields and serial, never a code or key.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Redeem handles one phase-enroll request. It is Service's enroll.Redeemer entry point for the
// foreground window; RedeemIntent serves the burst listener (intent-bound codes only).
func (s *Service) Redeem(ctx context.Context, req enroll.Request, src string) (enroll.Response, error) {
	return s.redeem(ctx, req, src, false)
}

// RedeemIntent is Redeem for the ServeWhileOffers listener: a code without a live intent is E659 no_intent.
func (s *Service) RedeemIntent(ctx context.Context, req enroll.Request, src string) (enroll.Response, error) {
	return s.redeem(ctx, req, src, true)
}

func (s *Service) redeem(ctx context.Context, req enroll.Request, src string, intentOnly bool) (enroll.Response, error) {
	s.mu.Lock()
	s.inflight++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.inflight--
		s.mu.Unlock()
		s.signal()
	}()
	now := s.cfg.CA.Now()
	if d := now.Sub(time.Unix(req.ClientTime, 0)); d > pki.ClockSkew || -d > pki.ClockSkew {
		return enroll.Response{}, &pki.Error{Reason: pki.ReasonClockSkew, Msg: "node and coordinator clocks differ by more than 120 s"}
	}
	name := strings.ToLower(req.Name)
	if !pki.ValidID(name) || len(req.NodeKey) != ed25519.PublicKeySize || !ValidShape(req.Code) {
		return enroll.Response{}, fail(ReasonBadRequest, "request is incomplete")
	}
	csrFP, err := pki.CSRKeyFingerprint(req.CSR)
	if err != nil {
		return enroll.Response{}, err
	}
	keyID := pki.NodeKeyID(req.NodeKey)
	if !ed25519.Verify(req.NodeKey, enroll.Transcript(s.cfg.CoordinatorID, req.ClientTime, csrFP, keyID), req.NodeKeySig) {
		return enroll.Response{}, &pki.Error{Reason: pki.ReasonNodeKeyProof, Msg: "the request is not signed by the node key it names"}
	}
	hash := HashCode(req.Code)
	if err = s.ownsLiveOffer(ctx, hash); err != nil { // an offer another process owns must not be burned here
		return enroll.Response{}, err
	}
	offer, res, err := s.cfg.Store.PKIOfferClaim(ctx, hash, intentOnly)
	if err != nil {
		return enroll.Response{}, err
	}
	if res != store.CodeOK {
		return enroll.Response{}, s.refuse(ctx, offer, res)
	}
	ap := s.approverFor(offer.ID)
	if ap == nil {
		_ = s.audit(ctx, "pairing.refuse", offer.ID, "reason="+ReasonNoApprover)
		return enroll.Response{}, fail(ReasonNoApprover, "offer %s has no approver in this process", offer.ID)
	}
	existing, err := s.vetExisting(ctx, name, req.NodeKey)
	if err == nil && existing && intentOnly {
		err = fail(ReasonExistingNode, "burst enrollment is for new nodes: %s is already registered", name)
	}
	if err != nil {
		reason := pki.ReasonOf(err)
		if reason == "" {
			reason = ReasonOf(err)
		}
		_ = s.audit(ctx, "pairing.refuse", offer.ID, fmt.Sprintf("node=%s csr=%s reason=%s", name, csrFP, reason))
		if reason == pki.ReasonNodeKeyChanged || reason == pki.ReasonRevoked || reason == ReasonExistingNode {
			s.alert(Alert{Kind: "pairing.takeover_attempt", OfferID: offer.ID, NodeID: name})
		}
		return enroll.Response{}, err
	}
	dctx, cancel := context.WithDeadline(ctx, time.Unix(0, offer.ExpiresAt).Add(time.Minute))
	defer cancel()
	dec, err := ap.Approve(dctx, ApprovalRequest{
		OfferID: offer.ID, Label: offer.Label, Intent: offer.Intent, NodeID: name, NodeName: req.Name, CSRFingerprint: csrFP, NodeKeyID: keyID,
		OS: req.OS, Arch: req.Arch, SourceAddr: src, CertTTL: time.Duration(offer.CertTTL), Existing: existing,
	})
	if err != nil || !dec.Approve {
		reason := dec.Reason
		if err != nil {
			reason = "approver error"
		}
		_ = s.audit(ctx, "pairing.deny", offer.ID, fmt.Sprintf("approver=%s node=%s csr=%s reason=%s", ap.Kind(), name, csrFP, cut(reason, 120)))
		return enroll.Response{}, fail(ReasonDenied, "enrollment of %s was not approved", name)
	}
	issued, err := s.enrollNode(ctx, offer, name, req, dec.Trust, existing)
	if err != nil {
		return enroll.Response{}, err
	}
	trust := "trust=kept"
	if !existing {
		trust = fmt.Sprintf("accepts=%s secrets=%s zone=%s projects=%s", strings.Join(dec.Trust.Accepts, ","), strings.Join(dec.Trust.SecretClasses, ","), dec.Trust.PrivacyZone, strings.Join(dec.Trust.Projects, ","))
	}
	if aerr := s.audit(ctx, "pairing.approve", offer.ID, fmt.Sprintf("approver=%s node=%s csr=%s serial=%s %s", ap.Kind(), name, csrFP, issued.Serial, trust)); aerr != nil {
		// A decision that cannot be audited is not honoured: fail closed and revoke what was just issued.
		_, _ = s.cfg.Store.PKIDeny(ctx, s.cfg.Actor, name, "audit_failed", issued.Serial)
		return enroll.Response{}, fmt.Errorf("pairing: the approval could not be audited; the certificate was revoked: %w", aerr)
	}
	s.mu.Lock()
	s.redeemed[offer.ID] = true
	s.mu.Unlock()
	s.signal()
	return enroll.Response{NodeID: name, Cert: issued.CertDER, Chain: issued.Chain, Root: issued.RootDER, AgentURL: s.cfg.AgentURL, NotAfter: issued.NotAfter.Unix(), ServerTime: s.cfg.CA.Now().Unix()}, nil
}

func cut(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (s *Service) audit(ctx context.Context, action, target, detail string) error {
	return s.cfg.Store.AppendAudit(ctx, s.cfg.Actor, action, target, detail)
}

// refuse maps a failed claim to E659 and, for a reused code, revokes the first certificate and alerts.
func (s *Service) refuse(ctx context.Context, o store.PKIOffer, res store.CodeClaim) error {
	switch res {
	case store.CodeReused:
		if o.Serial != "" {
			if _, err := s.cfg.Store.PKIDeny(ctx, s.cfg.Actor, o.NodeID, "code_reuse", o.Serial); err != nil {
				return fmt.Errorf("pairing: a used code was presented again but its certificate could not be revoked: %w", err)
			}
		}
		_ = s.audit(ctx, "pairing.reuse", o.ID, "revoked="+boolStr(o.Serial != ""))
		s.alert(Alert{Kind: "pairing.code_reuse", OfferID: o.ID, NodeID: o.NodeID, Serial: o.Serial})
		return fail(ReasonReused, "this code was already used; the certificate issued for it was revoked")
	case store.CodeExpired:
		return fail(ReasonExpired, "the code expired")
	case store.CodeLocked:
		return fail(ReasonLocked, "the code is locked after too many wrong attempts")
	case store.CodeWithdrawn:
		return fail(ReasonWithdrawn, "the offer was withdrawn")
	case store.CodeNoIntent:
		return fail(ReasonNoIntent, "this listener accepts codes bound to a live burst intent only")
	}
	return fail(ReasonInvalid, "the code is not valid")
}

func (s *Service) alert(a Alert) {
	if s.cfg.Alert != nil {
		s.cfg.Alert(a)
	}
}

// ownsLiveOffer refuses (without consuming anything) a code whose live offer has no approver in this
// process: it belongs to another process, which must be the one to redeem it.
func (s *Service) ownsLiveOffer(ctx context.Context, hash []byte) error {
	live, err := s.cfg.Store.PKIOffersLive(ctx, false)
	if err != nil {
		return err
	}
	for _, o := range live {
		if bytes.Equal(o.CodeHash, hash) && s.approverFor(o.ID) == nil {
			return fail(ReasonNoApprover, "this code belongs to a pairing window in another process")
		}
	}
	return nil
}

// vetExisting says whether the name is already registered. A registered node may re-pair only when it is
// not revoked and the joining node key equals its pin: a code holder cannot take over a node (an SSH
// node that never pinned a key included) by joining under its name.
func (s *Service) vetExisting(ctx context.Context, name string, key []byte) (bool, error) {
	node, err := s.cfg.Registry.Get(ctx, name)
	var se *store.Error
	if errors.As(err, &se) && se.Code == "E650" {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if node.Record.State == "revoked" {
		return true, &pki.Error{Reason: pki.ReasonRevoked, Msg: "node " + name + " is revoked"}
	}
	return true, s.cfg.CA.CheckPinnedNodeKey(ctx, name, key)
}

// enrollNode registers a new node (trust from the approver, key pinned) or keeps an existing one as it
// is, then issues the certificate and binds it to the offer.
func (s *Service) enrollNode(ctx context.Context, offer store.PKIOffer, name string, req enroll.Request, t TrustFields, existing bool) (*pki.Issued, error) {
	if !existing {
		if _, err := s.register(ctx, name, req, t); err != nil {
			return nil, err
		}
		if err := s.cfg.CA.PinNodeKey(ctx, name, req.NodeKey); err != nil {
			return nil, err
		}
	}
	issued, err := s.cfg.CA.IssueNode(ctx, pki.NodeRequest{CSR: req.CSR, NodeID: name, TTL: time.Duration(offer.CertTTL), OfferID: offer.ID})
	if err != nil {
		return nil, err
	}
	if tainted, berr := s.cfg.Store.PKIOfferBind(ctx, offer.ID, name, issued.Serial); berr != nil || tainted {
		_, _ = s.cfg.Store.PKIDeny(ctx, s.cfg.Actor, name, "code_reuse", issued.Serial)
		if s.approverFor(offer.ID) == nil { // Withdraw drops the approver
			return nil, fail(ReasonWithdrawn, "the offer was withdrawn while it was being approved; the certificate was revoked")
		}
		return nil, fail(ReasonReused, "this code was presented twice; the certificate was revoked")
	}
	if existing {
		if err = s.cfg.CA.PinNodeKey(ctx, name, req.NodeKey); err != nil { // finalizes a pending pin, only now that the offer is bound
			return nil, err
		}
	}
	if s.cfg.AfterIssue != nil {
		if err = s.cfg.AfterIssue(ctx, name); err != nil {
			return nil, err
		}
	}
	return issued, nil
}

func (s *Service) register(ctx context.Context, name string, req enroll.Request, t TrustFields) (registry.Node, error) {
	n, err := s.cfg.Registry.Register(ctx, draftCapability(name, req, s.cfg.CA.Now().UTC()), s.cfg.Actor)
	if err != nil {
		return n, err
	}
	admin, err := store.NewAdminAuthority(s.cfg.Actor)
	if err != nil {
		return n, err
	}
	if n, err = s.cfg.Registry.SetTrust(ctx, name, trustOf(t, s.cfg.CA.Now().UTC()), admin); err != nil {
		return n, fail(ReasonBadRequest, "trust fields are not valid: %v", err)
	}
	for _, p := range t.Projects {
		if n, err = s.cfg.Registry.Authorize(ctx, name, p, s.cfg.Actor); err != nil {
			return n, err
		}
	}
	return n, nil
}
