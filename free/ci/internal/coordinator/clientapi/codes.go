package clientapi

// Purpose: E664 "coordinator user API authorization refused" and its reason keys (Epic D15, P7-NODE-19):
// the one code of the user API and of the coordinator client.
// Inputs:  none.
// Outputs: *Error (code, reason key, message), Body (the JSON error body of a refused request), ReasonOf.
// Constraints: a message holds the reason and ids only, never a token, a certificate body or a header
//          value; reasons are stable keys the client and tests match on; the roles reasons (role,
//          project, no_grant, token_*) pass through unchanged so one key names one cause everywhere.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeUserAPI is E664.
const CodeUserAPI = "E664"

// Reason keys carried by Error.Reason.
const (
	ReasonNodeKind       = "node_kind"       // a node or coordinator certificate on the user API
	ReasonBadCertificate = "bad_certificate" // unreadable, expired or revoked user certificate
	ReasonNoCredential   = "no_credential"   // neither a user certificate nor a token
	ReasonTokenUnknown   = "token_unknown"
	ReasonTokenExpired   = "token_expired"
	ReasonTokenRevoked   = "token_revoked"
	ReasonUnscopedToken  = "unscoped_token" // a token that names no project
	ReasonNoGrant        = "no_grant"
	ReasonRole           = "role"
	ReasonProject        = "project"
	ReasonProjectBlocked = "project_not_allowed" // the project is not on the coordinator's allowlist
	ReasonPlainHTTP      = "plain_http"          // a token over a connection that is not TLS
	ReasonServerIdentity = "server_identity"     // client side: no pinned root, or the server leaf is not a coordinator's
	ReasonCredentialFile = "credential_file"     // client side: a credential or config file is missing or too open
	ReasonTransport      = "transport"           // client side: the coordinator could not be reached
	ReasonServeDown      = "serve_unreachable"   // client side: a local serve holds the lock but its socket does not answer
	ReasonBadRequest     = "bad_request"
	ReasonNotFound       = "not_found"
	ReasonConflict       = "conflict"
	ReasonInternal       = "internal"
)

func init() {
	model.Register(model.Code{ID: CodeUserAPI, Class: "usage", Summary: "coordinator user API refused the request or could not be used (reason key names why: node_kind, no_credential, token_*, role, project, server_identity, transport, ...)",
		Fix: "Use a user certificate or a scoped token that holds the needed role for the project (`nself ci serve admin roles list`), and pin the coordinator root (ci.coordinator_ca)"})
}

// Error is an E664 failure.
type Error struct{ Reason, Msg string }

func (e *Error) Error() string { return fmt.Sprintf("%s: %s (%s)", CodeUserAPI, e.Msg, e.Reason) }

// ErrCode and ErrReason let a handler map the error to its JSON body.
func (e *Error) ErrCode() string   { return CodeUserAPI }
func (e *Error) ErrReason() string { return e.Reason }

// Failf builds an E664 error.
func Failf(reason, format string, a ...any) error {
	return &Error{Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// ReasonOf returns the E664 reason key of err, or "" when err is not an E664 error.
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}

// Body is the JSON body of every error response.
type Body struct {
	Code    string `json:"code"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}
