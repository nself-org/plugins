// Package httpclient produces HTTP clients that automatically propagate the
// X-Request-ID stored on the request context by httpmid. Use New or Wrap
// instead of http.Client{} or &http.Client{} whenever a plugin makes a call
// to another plugin (or any internal service), so a single user request is
// traceable end-to-end.
//
// MIT licensed. Free counterpart of plugins-pro/paid/shared/httpclient (see
// free/shared/httpmid for why the two are deliberately separate modules).
package httpclient

import (
	"net/http"
	"time"

	"github.com/nself-org/plugins/free/shared-utils/httpmid"
)

// DefaultTimeout is used when Options.Timeout is zero.
const DefaultTimeout = 30 * time.Second

// Options configures a client produced by New.
type Options struct {
	// Timeout on the returned *http.Client. If zero, DefaultTimeout is used.
	Timeout time.Duration

	// Transport is the base RoundTripper wrapped by the request-ID transport.
	// If nil, http.DefaultTransport is used (a clone, to avoid mutating the
	// global).
	Transport http.RoundTripper
}

// New returns an *http.Client whose transport copies X-Request-ID from the
// outgoing request's context onto the wire header. If no request ID is set
// on the context, the outbound request is unchanged.
func New(opts Options) *http.Client {
	base := opts.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: &requestIDTransport{base: base},
	}
}

// Wrap returns a copy of c whose transport propagates X-Request-ID. The
// returned client shares nothing with the input except its Timeout. Passing
// a nil client returns a fresh one built from DefaultTransport.
func Wrap(c *http.Client) *http.Client {
	if c == nil {
		return New(Options{})
	}
	base := c.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	return &http.Client{
		Timeout:       c.Timeout,
		Transport:     &requestIDTransport{base: base},
		CheckRedirect: c.CheckRedirect,
		Jar:           c.Jar,
	}
}

// requestIDTransport attaches X-Request-ID from context to outbound requests
// before delegating to base.
type requestIDTransport struct {
	base http.RoundTripper
}

func (t *requestIDTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// If the caller already set an explicit X-Request-ID header, respect it.
	if req.Header.Get(httpmid.HeaderName) == "" {
		if id := httpmid.FromContext(req.Context()); id != "" {
			// Clone so we never mutate a caller-owned request header map when
			// one wasn't allocated yet.
			req = cloneRequestWithHeader(req, httpmid.HeaderName, id)
		}
	}
	return t.base.RoundTrip(req)
}

func cloneRequestWithHeader(req *http.Request, key, val string) *http.Request {
	r := req.Clone(req.Context())
	if r.Header == nil {
		r.Header = make(http.Header)
	}
	r.Header.Set(key, val)
	return r
}
