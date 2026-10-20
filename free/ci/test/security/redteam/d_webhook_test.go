//go:build security

package redteam

// Purpose: TM-33 red team, webhook half. Forged shared-secret deliveries against the GitHub trigger
// source: wrong, missing, malformed and re-encoded signatures, the shared-secret fallback for a
// repository that has its own secret, an empty HMAC key, and a replay of an old signed push after the
// ref moved or while the API is down.
// Inputs:  synthetic secrets, an in-process fake GitHub API over loopback TLS.
// Outputs: pass = the forged or replayed delivery is refused, or lands untrusted (control HELD).
// Constraints: no real network; every secret is fakeSecret derived.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/webhook"
	glwebhook "github.com/nself-org/plugins/free/ci/internal/providers/gitlab/webhook"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

const dRepoEnv = "D_REPO_SECRET"

func dSign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func dPush(repo string, id int, after string) []byte {
	return []byte(`{"ref":"refs/heads/main","before":"` + strings.Repeat("0", 40) + `","after":"` + after +
		`","repository":{"id":` + itoaD(id) + `,"full_name":"` + repo + `","private":true},"sender":{"login":"mallory","type":"User"}}`)
}

func itoaD(n int) string { return strconv.Itoa(n) }

type dRig struct {
	src  *webhook.Source
	api  *httptest.Server
	tip  string // what the fake ref API says the branch points at ("" = 404)
	down bool
}

func newDRig(t *testing.T, env map[string]string) *dRig {
	t.Helper()
	g := &dRig{}
	g.api = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if g.down || g.tip == "" || !strings.HasSuffix(r.URL.Path, "/git/ref/heads/main") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"object":{"type":"commit","sha":"` + g.tip + `"}}`))
	}))
	t.Cleanup(g.api.Close)
	g.src = &webhook.Source{
		Now:    func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) },
		Getenv: func(k string) string { return env[k] },
		Secrets: func(context.Context) (map[string]webhook.RepoSecret, error) {
			return map[string]webhook.RepoSecret{"o/r": {Env: dRepoEnv}}, nil
		},
		Insecure: func() bool { return false },
		NewAPI: func(context.Context) (*rest.Client, error) {
			return &rest.Client{BaseURL: g.api.URL, AllowPrivateBase: true, Provider: webhook.Name,
				TLSConfig: g.api.Client().Transport.(*http.Transport).TLSClientConfig}, nil
		},
	}
	return g
}

func dReq(sigHeaders []string, body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", "d-1")
	for _, s := range sigHeaders {
		req.Header.Add("X-Hub-Signature-256", s)
	}
	return req
}

// D10: nine malformed or forged signatures are all refused with E706; the honest one is accepted.
func TestD10_ForgedSignaturesRefused(t *testing.T) {
	secret := fakeSecret + "-repo"
	g := newDRig(t, map[string]string{dRepoEnv: secret, webhook.SharedSecretEnv: fakeSecret + "-shared"})
	body := dPush("o/r", 100, strings.Repeat("a", 40))
	good := dSign(secret, body)
	pretty := append([]byte(" "), body...) // same JSON, different bytes
	cases := map[string][]string{
		"missing header":            nil,
		"wrong secret":              {dSign("wrong", body)},
		"shared secret":             {dSign(fakeSecret+"-shared", body)},
		"empty key":                 {dSign("", body)},
		"signature of other bytes":  {dSign(secret, pretty)},
		"upper prefix":              {"SHA256=" + strings.TrimPrefix(good, "sha256=")},
		"trailing space":            {good + " "},
		"non-hex of right len":      {"sha256=" + strings.Repeat("z", 64)},
		"zero mac":                  {"sha256=" + strings.Repeat("0", 64)},
		"wrong first, right second": {dSign("wrong", body), good},
	}
	for name, hdr := range cases {
		if _, err := g.src.Verify(context.Background(), dReq(hdr, body), body); err == nil {
			t.Errorf("%s: forged delivery accepted", name)
		} else {
			var pe *providers.Error
			if !errors.As(err, &pe) || pe.Code != "E706" {
				t.Errorf("%s: want E706, got %v", name, err)
			}
		}
	}
	if _, err := g.src.Verify(context.Background(), dReq([]string{good}, body), body); err != nil {
		t.Fatalf("positive control: honest delivery refused: %v", err)
	}
}

// D11: the shared secret never authenticates a repository that has its own secret, and a repository
// authenticated by the shared secret is capped at untrusted whatever the facts say.
func TestD11_SharedSecretIsCappedAndNeverFallsBack(t *testing.T) {
	shared := fakeSecret + "-shared"
	g := newDRig(t, map[string]string{dRepoEnv: fakeSecret + "-repo", webhook.SharedSecretEnv: shared})
	own := dPush("o/r", 100, strings.Repeat("a", 40))
	if _, err := g.src.Verify(context.Background(), dReq([]string{dSign(shared, own)}, own), own); err == nil {
		t.Fatal("shared secret authenticated a repository with its own secret")
	}
	// case and whitespace variants of the name must not slip past the per-repo lookup onto the shared path
	for _, name := range []string{"O/R", "o/r ", " o/r", "o/r​", "o/R"} {
		b := dPush(name, 100, strings.Repeat("a", 40))
		f, err := g.src.Verify(context.Background(), dReq([]string{dSign(shared, b)}, b), b)
		if err == nil && f.Auth != "shared" {
			t.Errorf("name %q: shared-signed delivery got auth %q", name, f.Auth)
		}
		if err == nil && f.Auth == "shared" {
			if res := source.Classify(f); res.Class != model.TrustClass("untrusted") {
				t.Errorf("name %q: shared auth classified %s", name, res.Class)
			}
		}
	}
}

// D12: replay of an old signed push. The ref has moved: the delivery is declined. The API is down:
// the delivery goes on without HeadCurrent but cannot be owner on its own payload facts.
func TestD12_ReplayOfOldPush(t *testing.T) {
	secret := fakeSecret + "-repo"
	g := newDRig(t, map[string]string{dRepoEnv: secret})
	old := strings.Repeat("a", 40)
	body := dPush("o/r", 100, old)
	g.tip = strings.Repeat("b", 40)
	_, err := g.src.Verify(context.Background(), dReq([]string{dSign(secret, body)}, body), body)
	var d *webhook.Decline
	if !errors.As(err, &d) {
		t.Fatalf("replay after the ref moved: want a decline, got %v", err)
	}
	g.down = true
	f, err := g.src.Verify(context.Background(), dReq([]string{dSign(secret, body)}, body), body)
	if err != nil {
		t.Fatalf("API down: %v", err)
	}
	if res := source.Classify(f); res.Class == "owner" {
		t.Fatalf("an unverified replay classifies %s %v", res.Class, res.Reasons)
	}
	if f.HeadCurrent.Value != nil && *f.HeadCurrent.Value {
		t.Fatal("HeadCurrent claimed true with the API down")
	}
}

// D17: GitLab Standard Webhooks and token checks. Stale or future timestamps, a signature over another
// id, a project that has a signing secret receiving only its token, an empty configured token and a
// malformed signing secret are all refused with E706.
func TestD17_GitLabSignatureReplayAndFallback(t *testing.T) {
	key := []byte(fakeSecret + "-gitlab-key")
	secret := "whsec_" + base64.StdEncoding.EncodeToString(key)
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	env := map[string]string{"D_GL_SIGN": secret, "D_GL_TOKEN": fakeSecret + "-tok", "D_GL_EMPTY": "", "D_GL_BAD": "whsec_!!!"}
	src := &glwebhook.Source{
		Now: func() time.Time { return now }, Getenv: func(k string) string { return env[k] },
		Policy: func(context.Context) (glwebhook.Secrets, error) {
			return glwebhook.Secrets{
				Signing: map[string]glwebhook.RepoSecret{"g/signed": {Env: "D_GL_SIGN"}, "g/both": {Env: "D_GL_SIGN"}, "g/badkey": {Env: "D_GL_BAD"}},
				Tokens:  map[string]glwebhook.RepoSecret{"g/tok": {Env: "D_GL_TOKEN"}, "g/both": {Env: "D_GL_TOKEN"}, "g/empty": {Env: "D_GL_EMPTY"}},
			}, nil
		},
		NewAPI: func(context.Context) (*rest.Client, error) { return nil, errors.New("no api in this test") },
	}
	body := func(p string) []byte {
		return []byte(`{"object_kind":"push","ref":"refs/heads/main","before":"` + strings.Repeat("0", 40) + `","after":"` + strings.Repeat("a", 40) +
			`","project":{"id":5,"path_with_namespace":"` + p + `"}}`)
	}
	sign := func(id string, ts time.Time, b []byte) (string, string) {
		m := hmac.New(sha256.New, key)
		stamp := strconv.FormatInt(ts.Unix(), 10)
		m.Write([]byte(id + "." + stamp + "."))
		m.Write(b)
		return stamp, "v1," + base64.StdEncoding.EncodeToString(m.Sum(nil))
	}
	req := func(b []byte, h map[string]string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(b))
		r.Header.Set("X-Gitlab-Event", "Push Hook")
		for k, v := range h {
			r.Header.Set(k, v)
		}
		return r
	}
	b := body("g/signed")
	okStamp, okSig := sign("msg-1", now, b)
	cases := map[string]*http.Request{
		"stale by 301 s": nil, "future by 301 s": nil,
		"other id":         req(b, map[string]string{"webhook-id": "msg-2", "webhook-timestamp": okStamp, "webhook-signature": okSig}),
		"token only":       req(b, map[string]string{"X-Gitlab-Token": fakeSecret + "-tok"}),
		"bad signing key":  req(body("g/badkey"), map[string]string{"webhook-id": "msg-1", "webhook-timestamp": okStamp, "webhook-signature": okSig}),
		"empty token":      req(body("g/empty"), map[string]string{"X-Gitlab-Token": ""}),
		"both: token only": req(body("g/both"), map[string]string{"X-Gitlab-Token": fakeSecret + "-tok"}),
		"unlisted project": req(body("g/other"), map[string]string{"X-Gitlab-Token": fakeSecret + "-tok"}),
	}
	for name, off := range map[string]time.Duration{"stale by 301 s": -301 * time.Second, "future by 301 s": 301 * time.Second} {
		stamp, sig := sign("msg-1", now.Add(off), b)
		cases[name] = req(b, map[string]string{"webhook-id": "msg-1", "webhook-timestamp": stamp, "webhook-signature": sig})
	}
	for name, r := range cases {
		_, err := src.Verify(context.Background(), r, bodyOf(t, r))
		var pe *providers.Error
		if err == nil || !errors.As(err, &pe) || pe.Code != "E706" {
			t.Errorf("%s: want E706, got %v", name, err)
		}
	}
	good := req(b, map[string]string{"webhook-id": "msg-1", "webhook-timestamp": okStamp, "webhook-signature": okSig})
	if _, err := src.Verify(context.Background(), good, b); err != nil {
		var d *glwebhook.Decline
		if !errors.As(err, &d) {
			t.Fatalf("positive control: honest signed delivery refused: %v", err)
		}
	}
}

func bodyOf(t *testing.T, r *http.Request) []byte {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
