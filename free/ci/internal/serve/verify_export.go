// Package serve — verify_export.go
//
// Purpose: the one exported surface of the frozen v1.4 serve for the V15 coordinator (P7-NODE-10,
//
//	Epic D7): the HMAC verifier, the repository allowlist and the event parser, so the coordinator
//	reuses them instead of copying them.
//
// Inputs:  the same arguments the unexported functions take.
// Outputs: the same results; each wrapper only calls its original.
// Constraints: no behaviour change (the v1.4 serve stays frozen, CANON D11); additive file only.
package serve

import "errors"

// WebhookEvent is a parsed push or pull_request event.
type WebhookEvent struct {
	RepoFullName string // "owner/repo"
	CloneURL     string
	Ref          string // "refs/heads/main"
	SHA          string
	EventType    string // "push" | "pull_request"
}

// ErrSkipEvent marks an event the v1.4 serve acknowledges and does not run.
var ErrSkipEvent = errors.New("event skipped")

// skipError carries the v1.4 reason text and matches ErrSkipEvent.
type skipError struct{ err error }

func (e skipError) Error() string        { return e.err.Error() }
func (e skipError) Is(target error) bool { return target == ErrSkipEvent }

// VerifySignature checks an X-Hub-Signature-256 header ("sha256=<hex>") against body with secret.
func VerifySignature(secret, header string, body []byte) error {
	return verifySignature(secret, header, body)
}

// RepoAllowed reports whether repoFullName ("owner/repo") is in allowed; an empty list allows nothing.
func RepoAllowed(repoFullName string, allowed []string) bool {
	return isRepoAllowed(repoFullName, allowed)
}

// ParseWebhook parses a GitHub webhook body. An event the v1.4 serve skips (other event types, branch
// deletes, other pull_request actions) returns an error that matches ErrSkipEvent (errors.Is) and whose text is the v1.4 reason.
func ParseWebhook(event string, body []byte) (WebhookEvent, error) {
	j, err := parseEvent(event, body)
	if err != nil {
		return WebhookEvent{}, skipError{err}
	}
	return WebhookEvent{RepoFullName: j.RepoFullName, CloneURL: j.CloneURL, Ref: j.Ref, SHA: j.SHA, EventType: j.EventType}, nil
}

// CloneRef shallow-clones cloneURL at sha into a new directory under workDir (which must exist) and
// returns the directory; on error the directory is still returned so the caller can remove it.
func CloneRef(cloneURL, sha, workDir string) (string, error) {
	return cloneRef(ciJob{CloneURL: cloneURL, SHA: sha}, ServeConfig{WorkDir: workDir})
}
