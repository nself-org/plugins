package webhook

// Purpose: the errors a GitLab trigger source returns (Epic D11, D15). A verification failure is a plain
// providers.Error E706 with a reason key (signature, token, timestamp): the ingress adapter passes it
// up and the coordinator answers 403 with nothing enqueued. A verified delivery that must not run (an
// event nobody serves, a stale replay) is a *Decline: the coordinator answers 200 {"accepted": false}.
// Inputs:  a reason key (signature, token, timestamp, stale, unsupported) and safe text.
// Outputs: error values; no payload bytes, header values or secrets ever enter their text.
// Constraints: error codes come only from E700-E709.

import (
	"net/http"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// Reason keys of E706.
const (
	reasonSignature   = "signature"
	reasonToken       = "token"
	reasonTimestamp   = "timestamp"
	reasonStale       = "stale"
	reasonUnsupported = "unsupported"
)

func reject(reason string) error {
	return providers.NewError(providers.Auth, "E706", reason, http.StatusForbidden, time.Time{})
}

// errSignature is the 403 of a delivery whose signature is missing, wrong or not for this project.
func errSignature() error { return reject(reasonSignature) }

// errToken is the 403 of a bare X-Gitlab-Token that is missing or wrong.
func errToken() error { return reject(reasonToken) }

// errTimestamp is the 403 of a correctly signed delivery whose timestamp is outside the window.
func errTimestamp() error { return reject(reasonTimestamp) }

// Decline is a verified delivery that is acknowledged and not enqueued.
type Decline struct {
	Reason string           // safe text sent back to GitLab
	Err    *providers.Error // E706 with reason unsupported or stale
}

func (d *Decline) Error() string         { return "delivery declined: " + d.Reason }
func (d *Decline) DeclineReason() string { return d.Reason }
func (d *Decline) Unwrap() error         { return d.Err }

func decline(key, text string) error {
	return &Decline{Reason: text, Err: providers.NewError(providers.Config, "E706", key, http.StatusOK, time.Time{})}
}

func unsupported(text string) error { return decline(reasonUnsupported, text) }
