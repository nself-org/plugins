package sdk

import (
	"net/http"
	"strings"
)

// pluginTransport is an http.RoundTripper that injects inter-plugin identity
// headers on every outbound request. S43-T16.
//
// It always sets:
//   - X-Source-Plugin: <pluginName>  (for AllowedCallers middleware on the receiver)
//
// If an identity is provided, it additionally calls id.SignRequest() which sets:
//   - X-Plugin-Id: <pluginName>
//   - X-Plugin-Timestamp: <unix-ts>
//   - X-Plugin-Signature: <ed25519-sig>
type pluginTransport struct {
	pluginName string
	identity   *PluginIdentity // nil = X-Source-Plugin only, no signature
	base       http.RoundTripper
}

// RoundTrip implements http.RoundTripper. It clones the request, injects
// headers, then delegates to the underlying transport.
func (t *pluginTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone to avoid mutating the caller's request.
	r := req.Clone(req.Context())
	if r.Header == nil {
		r.Header = make(http.Header)
	}

	// Normalise and set X-Source-Plugin.
	name := strings.ToLower(strings.TrimSpace(t.pluginName))
	r.Header.Set("X-Source-Plugin", name)

	// Optionally sign the request with Ed25519 identity headers.
	if t.identity != nil {
		t.identity.SignRequest(r)
	}

	return t.base.RoundTrip(r)
}

// NewPluginClient returns an *http.Client whose transport automatically
// injects X-Source-Plugin: <pluginName> on every outgoing request. This
// satisfies the AllowedCallers middleware on the receiving plugin's side.
//
// If identity is non-nil, the client also signs each request with Ed25519
// headers (X-Plugin-Id, X-Plugin-Timestamp, X-Plugin-Signature).
//
// Usage:
//
//	client := sdk.NewPluginClient("claw", nil)            // header only
//	client := sdk.NewPluginClient("claw", myIdentity)     // header + signature
//	resp, err := client.Get("http://ai:3001/api/complete")
//
// S43-T16.
func NewPluginClient(pluginName string, identity *PluginIdentity) *http.Client {
	base := http.DefaultTransport
	if base == nil {
		base = http.DefaultTransport
	}
	return &http.Client{
		Transport: &pluginTransport{
			pluginName: pluginName,
			identity:   identity,
			base:       base,
		},
	}
}
