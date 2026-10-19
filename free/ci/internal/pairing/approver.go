package pairing

// Purpose: the per-offer Approver (Epic D5): the decision that turns a redeemed code into a certificate.
// Inputs:  an ApprovalRequest naming the node, the CSR key fingerprint the node printed, the code
//          metadata (label, intent) and the source address.
// Outputs: a Decision: approve with trust fields, or deny with a reason.
// Constraints: an Approver runs only in the process that owns the enrollment listener on the coordinator
//          host; one Approver belongs to one offer and is never shared with another; it never receives
//          the code; a denial or an error issues nothing.

import (
	"context"
	"time"
)

// ApprovalRequest is what an Approver sees.
type ApprovalRequest struct {
	OfferID, Label, Intent string
	NodeID, NodeName       string
	// CSRFingerprint is SHA256:<base64> of the CSR key SPKI; NodeKeyID the Ed25519 key id.
	CSRFingerprint, NodeKeyID string
	OS, Arch, SourceAddr      string
	// Existing is true when the node name is already registered: the node key matched its pin and the
	// registered trust fields are kept (the Decision's trust fields are not applied).
	Existing bool
	CertTTL  time.Duration
}

// TrustFields are the coordinator-assigned trust fields of the new node (ADR 0025 §2).
type TrustFields struct {
	Accepts, SecretClasses []string
	PrivacyZone            string
	// Projects are authorized for the node (empty: none yet).
	Projects []string
}

// Decision is an Approver's answer. Reason is kept in the audit row (cut to 120 characters).
type Decision struct {
	Approve bool
	Reason  string
	Trust   TrustFields
}

// Approver decides one offer.
type Approver interface {
	// Kind names the approver in audit rows: "admin" for the prompt, "policy:<name>" for a policy.
	Kind() string
	Approve(ctx context.Context, r ApprovalRequest) (Decision, error)
}

// DefaultTrust is ADR 0025 §2 for personal and LAN nodes: accepts owner and internal, no secrets.
func DefaultTrust() TrustFields {
	return TrustFields{Accepts: []string{"owner", "internal"}, SecretClasses: []string{"none"}}
}
