package checks

// Purpose: the installation token source. It signs a JWT, exchanges it for a short-lived
// installation token through the REST funnel, keeps the token in memory until 60 s before it
// expires and asks again after that.
// Inputs:  App id, installation id, the private key, a client factory and a clock.
// Outputs: an AuthFunc for the installation client; Token() for tests.
// Constraints: the token and the JWT are never logged, stored or put in an error: every value is
// registered with the redactor, which masks error text. The exchange asks for exactly the four
// permissions this adapter needs (checks:write, statuses:write, contents:read, metadata:read);
// GitHub never grants more than the App has, and a token with fewer is not an error. A refresh is
// serialised, so concurrent callers share one exchange.

import (
	"context"
	"crypto/rsa"
	"net/url"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// refreshMargin is how long before expiry a cached token is replaced.
const refreshMargin = 60 * time.Second

// defaultLifetime is assumed when GitHub sends no usable expiry (the real one is 1 hour).
const defaultLifetime = 5 * time.Minute

// API is the part of the REST funnel the adapter uses; *rest.Client satisfies it.
type API interface {
	DoQuery(ctx context.Context, method, pathTemplate string, params map[string]string, query url.Values, body, out any) (rest.Response, error)
}

// ClientFactory builds a REST client that sends auth as the bearer token.
type ClientFactory func(auth rest.AuthFunc) (API, error)

// TokenSource caches one installation token.
type TokenSource struct {
	AppID, InstallationID string
	Repo                  string // the repository the token is limited to ("" = every repository of the installation)
	Key                   *rsa.PrivateKey
	Client                ClientFactory
	Now                   func() time.Time
	Red                   *redact.Redactor

	mu      sync.Mutex
	token   string
	expires time.Time
}

func (t *TokenSource) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

type tokenReply struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// permissions is the whole request: nothing beyond these four is ever asked for.
var permissions = map[string]string{"checks": "write", "statuses": "write", "contents": "read", "metadata": "read"}

// Token returns a valid installation token, exchanging a fresh JWT when none is cached or the
// cached one is within refreshMargin of its expiry.
func (t *TokenSource) Token(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if t.token != "" && now.Before(t.expires.Add(-refreshMargin)) {
		return t.token, nil
	}
	jwt, err := SignJWT(t.AppID, t.Key, now)
	if err != nil {
		return "", err
	}
	registerValue(t.Red, []byte(jwt))
	jwtAuth := func(context.Context) (string, error) { return jwt, nil }
	req := map[string]any{"permissions": permissions}
	if t.Repo != "" { // least privilege: the token works on the one repository being published
		req["repositories"] = []string{t.Repo}
	}
	c, err := t.Client(jwtAuth)
	if err != nil {
		return "", coded("the App client could not be built")
	}
	var reply tokenReply
	_, err = c.DoQuery(ctx, "POST", "/app/installations/{id}/access_tokens", map[string]string{"id": t.InstallationID}, nil, req, &reply)
	if err != nil {
		return "", coded("the installation token was not obtained: " + safeText(t.Red, err.Error()))
	}
	if reply.Token == "" {
		return "", coded("the installation token was not obtained: empty answer")
	}
	registerValue(t.Red, []byte(reply.Token))
	t.expires = reply.ExpiresAt
	if reply.ExpiresAt.IsZero() || !reply.ExpiresAt.After(now) {
		t.expires = now.Add(defaultLifetime)
	}
	t.token = reply.Token
	return t.token, nil
}

// Auth is the AuthFunc of the installation client.
func (t *TokenSource) Auth() rest.AuthFunc { return t.Token }
