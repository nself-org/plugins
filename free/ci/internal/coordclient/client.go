package coordclient

// Purpose: the HTTP client of the coordinator user API (P7-NODE-19, Epic D14): TLS 1.3 with the pinned
// coordinator root and a user certificate or a scoped token, or the unix socket of a local serve.
// Inputs:  a Target and the credential files it names.
// Outputs: a Client whose methods return clientapi errors (E664 with a reason key).
// Constraints: the root comes from `ci.coordinator_ca` (never the system roots) and the server leaf must
//          be a coordinator-kind certificate, else E664 server_identity; a token is read from a 0600 file,
//          sent only as a bearer header over TLS and never printed, logged or put in an error; the client
//          imports no store, so it cannot open the database.

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/pki"
)

// Client talks to one coordinator.
type Client struct {
	base  string
	hc    *http.Client
	token string
}

// New builds the client of a target (Remote or LocalServe).
func New(t Target) (*Client, error) {
	switch t.Kind {
	case LocalServe:
		if !SocketIsOurs(t.Socket) {
			return nil, clientapi.Failf(clientapi.ReasonServeDown, "the local serve holds the lock but %s is not its socket (missing, a link, or another user's)", t.Socket)
		}
		tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", t.Socket)
		}}
		return &Client{base: "http://serve", hc: &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
	case Remote:
		return newRemote(t)
	}
	return nil, clientapi.Failf(clientapi.ReasonBadRequest, "this target has no coordinator")
}

func newRemote(t Target) (*Client, error) {
	u, err := url.Parse(t.URL)
	switch {
	case err != nil || u.Host == "":
		return nil, clientapi.Failf(clientapi.ReasonBadRequest, "the coordinator address is not a URL")
	case u.Scheme != "https":
		return nil, clientapi.Failf(clientapi.ReasonPlainHTTP, "the coordinator address must be https; credentials never travel in clear text")
	}
	root, err := readRoot(t.Creds.CA)
	if err != nil {
		return nil, err
	}
	opts := pki.ClientOptions{RootDER: root, ServerName: u.Hostname()}
	c := &Client{base: t.URL}
	if t.Creds.Cert != "" || t.Creds.Key != "" {
		pair, err := tls.LoadX509KeyPair(t.Creds.Cert, t.Creds.Key)
		if err != nil {
			return nil, clientapi.Failf(clientapi.ReasonCredentialFile, "the user certificate or key cannot be read")
		}
		opts.Cert = &pair
	}
	if t.Creds.TokenFile != "" {
		if c.token, err = readSecretFile(t.Creds.TokenFile); err != nil {
			return nil, err
		}
	}
	if opts.Cert == nil && c.token == "" {
		return nil, clientapi.Failf(clientapi.ReasonNoCredential, "configure a user certificate (ci.coordinator_cert, ci.coordinator_key) or a token file (ci.coordinator_token_file)")
	}
	cfg, err := pki.TLSClientConfig(opts)
	if err != nil {
		return nil, clientapi.Failf(clientapi.ReasonServerIdentity, "the pinned coordinator root is not a certificate")
	}
	c.hc = &http.Client{Transport: &http.Transport{TLSClientConfig: cfg, ResponseHeaderTimeout: 30 * time.Second},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }} // the API never redirects: a bearer token must not follow one
	return c, nil
}

// readRoot returns the DER of the first certificate in the pinned root file.
func readRoot(path string) ([]byte, error) {
	if path == "" {
		return nil, clientapi.Failf(clientapi.ReasonServerIdentity, "ci.coordinator_ca is not set; pin the coordinator root from your user bundle")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, clientapi.Failf(clientapi.ReasonServerIdentity, "the pinned coordinator root cannot be read")
	}
	for {
		var b *pem.Block
		if b, raw = pem.Decode(raw); b == nil {
			return nil, clientapi.Failf(clientapi.ReasonServerIdentity, "the pinned coordinator root holds no certificate")
		}
		if b.Type == "CERTIFICATE" {
			return b.Bytes, nil
		}
	}
}

// readSecretFile reads a one-line secret from a regular file only its owner can touch.
func readSecretFile(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o077 != 0 {
		return "", clientapi.Failf(clientapi.ReasonCredentialFile, "the token file must be a regular file with mode 0600")
	}
	raw, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(raw)) == "" {
		return "", clientapi.Failf(clientapi.ReasonCredentialFile, "the token file is empty or unreadable")
	}
	return strings.TrimSpace(string(raw)), nil
}

// do sends one request. A non-2xx answer is returned as the E664 error its body carries.
func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return nil, clientapi.Failf(clientapi.ReasonBadRequest, "the request cannot be built")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, netError(err)
	}
	if resp.StatusCode/100 == 2 {
		return resp, nil
	}
	defer func() { _ = resp.Body.Close() }()
	var b clientapi.Body
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&b) != nil || b.Code != clientapi.CodeUserAPI {
		return nil, clientapi.Failf(clientapi.ReasonTransport, "the coordinator answered %d", resp.StatusCode)
	}
	return nil, &clientapi.Error{Reason: b.Reason, Msg: b.Message}
}

// netError maps a transport failure to its reason key: a server that is not the pinned coordinator is
// server_identity, a refused client certificate is bad_certificate, anything else is transport.
func netError(err error) error {
	var pe *pki.Error
	var cv *tls.CertificateVerificationError
	var ua x509.UnknownAuthorityError
	var he x509.HostnameError
	var ci x509.CertificateInvalidError
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.As(err, &pe), errors.As(err, &cv), errors.As(err, &ua), errors.As(err, &he), errors.As(err, &ci):
		return clientapi.Failf(clientapi.ReasonServerIdentity, "the server is not the pinned coordinator")
	case strings.Contains(err.Error(), "bad certificate"), strings.Contains(err.Error(), "certificate required"):
		return clientapi.Failf(clientapi.ReasonBadCertificate, "the coordinator refused the user certificate")
	}
	inner := errors.Unwrap(err)
	if inner == nil {
		inner = err
	}
	return clientapi.Failf(clientapi.ReasonTransport, "cannot reach the coordinator: %s", inner)
}

// getJSON GETs path and decodes the JSON answer into v.
func (c *Client) getJSON(ctx context.Context, path string, v any) error {
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(v) != nil {
		return clientapi.Failf(clientapi.ReasonTransport, "the coordinator answer is not JSON")
	}
	return nil
}
