//go:build security

package redteam

// Purpose: isolating regression tests added after the mutation review of the HELD claims (P7-TRUST-18),
// webhook half (the empty-secret guard of the GitHub signature check, the shared-secret trust cap) and SSRF half
// (the private-base allowance at dial level), each on a fixture that no other rule also refuses.
// Inputs:  the dRig GitHub source of d_webhook_test.go, source.Classify.
// Outputs: pass (control held) or fail (attack worked).
// Constraints: no real network; every secret is fakeSecret derived.

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/webhook"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

// D10b: a repository whose secret variable is set but EMPTY. HMAC under an empty key is computable by
// anyone, so a signature made with "" must be refused (the empty-secret guard), and the real secret of
// another repository must not open it either. The control is a second repository with a real secret.
func TestD10_EmptyRepoSecretIsNeverAKey(t *testing.T) {
	const otherEnv = "D_OTHER_SECRET"
	env := map[string]string{dRepoEnv: "", otherEnv: fakeSecret + "-other"}
	g := newDRig(t, env)
	prev := g.src.Secrets
	g.src.Secrets = func(ctx context.Context) (map[string]webhook.RepoSecret, error) {
		m, err := prev(ctx)
		m["o/other"] = webhook.RepoSecret{Env: otherEnv}
		return m, err
	}
	good := dPush("o/other", 7, strings.Repeat("a", 40))
	if _, err := g.src.Verify(context.Background(), dReq([]string{dSign(fakeSecret+"-other", good)}, good), good); err != nil {
		t.Fatalf("control: a repository with a real secret refused its honest delivery: %v", err)
	}
	body := dPush("o/r", 100, strings.Repeat("a", 40))
	for name, sig := range map[string]string{"signed with the empty key": dSign("", body), "signed with another repo secret": dSign(fakeSecret+"-other", body)} {
		_, err := g.src.Verify(context.Background(), dReq([]string{sig}, body), body)
		var pe *providers.Error
		if err == nil || !errors.As(err, &pe) || pe.Code != "E706" {
			t.Errorf("%s: want E706 for a repository whose secret is empty, got %v", name, err)
		}
	}
}

// D11b: a delivery signed with the SHARED secret for a repository that has no secret of its own, sent by
// someone who really is an admin of it, is still capped at untrusted: the shared secret proves nothing
// about the repository. The same facts under a per-repo secret are the control (owner).
func TestD11_SharedAuthCapsAnAdminSender(t *testing.T) {
	shared := fakeSecret + "-shared"
	g := newDRig(t, map[string]string{dRepoEnv: fakeSecret + "-repo", webhook.SharedSecretEnv: shared})
	after := strings.Repeat("a", 40)
	g.tip = after
	body := dPush("o/unlisted", 5, after)
	f, err := g.src.Verify(context.Background(), dReq([]string{dSign(shared, body)}, body), body)
	if err != nil || f.Auth != "shared" {
		t.Fatalf("setup: a shared-signed delivery must verify as shared: auth %q err %v", f.Auth, err)
	}
	yes := true
	admin := "admin"
	f.ActorPermission = model.Fact[string]{Value: &admin, Source: "api"}
	f.ProtectedRef = model.Fact[bool]{Value: &yes, Source: "api"}
	f.Actor = model.Fact[string]{Value: &admin, Source: "api"}
	f.Bot, f.Fork = model.Fact[bool]{}, model.Fact[bool]{}
	per := f
	per.Auth = "per-repo"
	if res := source.Classify(per); res.Class != "owner" {
		t.Fatalf("control: with a per-repo secret these facts must classify owner, got %s %v", res.Class, res.Reasons)
	}
	res := source.Classify(f)
	if res.Class != "untrusted" || !slices.Contains(res.Reasons, source.ReasonAuthNotPerRepo) {
		t.Errorf("shared-secret delivery from an admin classified %s %v, want untrusted with %s", res.Class, res.Reasons, source.ReasonAuthNotPerRepo)
	}
}

// dCountListener counts accepted TCP connections: a dial that reached the listener is counted even when the
// TLS handshake or the request then fails, so a name or certificate mismatch cannot hide a private dial.
type dCountListener struct {
	net.Listener
	n atomic.Int32
}

func (l *dCountListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		l.n.Add(1)
	}
	return c, err
}

// dLocalhostServer starts a loopback TLS server that counts connections. The client is addressed as
// "localhost" (a name, so the download and redirect host rules apply) and verifies the httptest certificate
// under its example.com name.
func dLocalhostServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *dCountListener, *tls.Config, string) {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	cl := &dCountListener{Listener: srv.Listener}
	srv.Listener = cl
	srv.StartTLS()
	t.Cleanup(srv.Close)
	cfg := srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	cfg.ServerName = "example.com"
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	return srv, cl, cfg, port
}

// D13b/D14b/D15b: the private-base allowance is for the first hop of an API call or a download to the
// base's own authority and nothing else. Counted at TCP accept, on a base that is a NAME (so the host rules apply and the
// allowance is exercised), with AllowPrivateBase on.
func TestD13_PrivateBaseAllowanceIsFirstHopOnly(t *testing.T) {
	_, canaryConns, _, canaryPort := dLocalhostServer(t, nil)
	var redirectTo string
	_, forgeConns, cfg, forgePort := dLocalhostServer(t, func(w http.ResponseWriter, r *http.Request) {
		if redirectTo != "" && r.URL.Path == "/start" {
			http.Redirect(w, r, redirectTo, http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("{}"))
	})
	c := &rest.Client{BaseURL: "https://localhost:" + forgePort, Provider: "github", AllowPrivateBase: true, TLSConfig: cfg, RedirectHosts: []string{"localhost"}}
	if _, err := c.Do(context.Background(), "GET", "/ping", nil, nil, nil); err != nil || forgeConns.n.Load() != 1 {
		t.Fatalf("control: the first hop to the private base must work: err=%v conns=%d", err, forgeConns.n.Load())
	}
	// redirect to the SAME host name on another port: the allowance is not inherited
	redirectTo = "https://localhost:" + canaryPort + "/steal"
	if _, err := c.Do(context.Background(), "GET", "/start", nil, nil, nil); err == nil {
		t.Error("a redirect from the private base to the same host name on another port succeeded")
	}
	if n := canaryConns.n.Load(); n != 0 {
		t.Errorf("a redirect dialed the private address (%d connection(s))", n)
	}
	// D-1344: a download from the base's own authority (same host and port) is a first hop of the same
	// kind as an API call, so with the operator's opt-in it may reach the private base. Nothing else
	// inherits the allowance: not the same host name on another port, not a redirect, not a client
	// without the opt-in.
	before := forgeConns.n.Load()
	var sink bytes.Buffer
	if _, err := c.Download(context.Background(), "https://localhost:"+forgePort+"/artifact", 1024, &sink); err != nil || forgeConns.n.Load() != before+1 {
		t.Errorf("a download from the opted-in private base's own authority must work: err=%v conns=%d", err, forgeConns.n.Load()-before)
	}
	if _, err := c.Download(context.Background(), "https://localhost:"+canaryPort+"/artifact", 1024, &sink); err == nil {
		t.Error("a download from the base host name on another port succeeded")
	}
	if n := canaryConns.n.Load(); n != 0 {
		t.Errorf("a download dialed the private address on another port (%d connection(s))", n)
	}
	redirectTo = "https://localhost:" + canaryPort + "/steal"
	if _, err := c.Download(context.Background(), "https://localhost:"+forgePort+"/start", 1024, &sink); err == nil {
		t.Error("a download redirected to another port of the private base host succeeded")
	}
	if n := canaryConns.n.Load(); n != 0 {
		t.Errorf("a download redirect dialed the private address (%d connection(s))", n)
	}
	closed := &rest.Client{BaseURL: "https://localhost:" + forgePort, Provider: "github", TLSConfig: cfg, RedirectHosts: []string{"localhost"}}
	before = forgeConns.n.Load()
	if _, err := closed.Download(context.Background(), "https://localhost:"+forgePort+"/artifact", 1024, &sink); err == nil {
		t.Error("a download from a private base without the opt-in succeeded")
	}
	if n := forgeConns.n.Load(); n != before {
		t.Errorf("a download without the opt-in dialed the private base (%d new connection(s))", n-before)
	}
}
