package pki

// Purpose: E660 "certificate invalid" and its reason keys (Epic D15), the one error type of this package.
// Inputs:  none.
// Outputs: *Error with a stable reason key; the code registered through model.Register.
// Constraints: a message holds the reason and ids only, never a key, a code, a token or a certificate body.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeInvalid is E660: a certificate, key pin, fingerprint, clock or token was refused.
const CodeInvalid = "E660"

// Reason keys carried by Error.Reason.
const (
	ReasonRevoked                = "revoked"
	ReasonExpired                = "expired"
	ReasonWrongKind              = "wrong_kind"
	ReasonBadChain               = "bad_chain"
	ReasonNoCert                 = "no_certificate"
	ReasonUnknownNode            = "unknown_node"
	ReasonClockSkew              = "clock_skew"
	ReasonCertTTL                = "cert_ttl"
	ReasonRenewCap               = "renew_cap"
	ReasonBadCSR                 = "bad_csr"
	ReasonNodeKeyChanged         = "node_key_changed"
	ReasonFingerprintMismatch    = "fingerprint_mismatch"
	ReasonFingerprintUnconfirmed = "fingerprint_unconfirmed"
	ReasonTokenSignature         = "token_signature"
	ReasonTokenExpired           = "token_expired"
	ReasonTokenUnknownKey        = "token_unknown_key"
	ReasonTokenMalformed         = "token_malformed"
)

func init() {
	model.Register(model.Code{ID: CodeInvalid, Class: "usage", Summary: "certificate invalid", Fix: "Re-pair the node (`nself ci nodes pair`), check the clock, or issue a new certificate"})
}

// Error is an E660 failure.
type Error struct{ Reason, Msg string }

func (e *Error) Error() string { return fmt.Sprintf("%s: %s (%s)", CodeInvalid, e.Msg, e.Reason) }

// ErrCode and ErrReason let the enrollment handler map the error to its JSON body.
func (e *Error) ErrCode() string   { return CodeInvalid }
func (e *Error) ErrReason() string { return e.Reason }

func fail(reason, format string, a ...any) error {
	return &Error{Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// ReasonOf returns the E660 reason key of err, or "" when err is not an E660 error.
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}
