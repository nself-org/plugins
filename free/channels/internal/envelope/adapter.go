package envelope

import (
	"context"
	"net/http"
)

// Adapter is the service provider interface every channel adapter implements.
//
// An adapter translates between one provider's wire format and the
// channels.v1 Envelope. It sets origin-trust tags it can observe but never
// decides what to do with them.
type Adapter interface {
	// Capabilities reports what the provider supports.
	Capabilities() Capabilities
	// Send delivers an outbound envelope and returns its receipt.
	Send(ctx context.Context, env Envelope) (Receipt, error)
	// Edit replaces the content of a message already sent under
	// providerMessageID with the parts of env.
	Edit(ctx context.Context, providerMessageID string, env Envelope) (Receipt, error)
	// VerifyInbound checks the provider signature or token on an inbound
	// request. A non-nil error means the request must be rejected.
	VerifyInbound(header http.Header, body []byte) error
	// ParseInbound turns a verified inbound request into zero or more envelopes.
	ParseInbound(ctx context.Context, header http.Header, body []byte) ([]Envelope, error)
}
