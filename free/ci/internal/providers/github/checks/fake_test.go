package checks

// Test kit: a recording fake of the REST funnel that answers from recorded GitHub bodies
// (testdata/*.json). It runs the AuthFunc it was built with, so the tests see which bearer each
// request would carry, and it verifies the App JWT like GitHub would.

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

const (
	testSHA   = "0123456789abcdef0123456789abcdef01234567"
	testToken = "fixture-installation-token-0001"
)

type call struct {
	Method, Path string
	Params       map[string]string
	Query        url.Values
	Body         []byte
	Bearer       string
}

type fakeGH struct {
	mu       sync.Mutex
	calls    []call
	list     string // fixture served for the list call
	expires  string // expires_at override for the token reply ("" keeps the fixture)
	failPath string // a path template that answers with failErr
	failErr  error
	pub      *rsa.PublicKey
	t        *testing.T
	clients  int
	now      func() time.Time
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newFake(t *testing.T, key *rsa.PrivateKey) *fakeGH {
	return &fakeGH{t: t, list: "list-empty.json", pub: &key.PublicKey}
}

func (f *fakeGH) factory() ClientFactory {
	return func(auth rest.AuthFunc) (API, error) {
		f.mu.Lock()
		f.clients++
		f.mu.Unlock()
		return &fakeClient{f: f, auth: auth}, nil
	}
}

func (f *fakeGH) snapshot() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]call(nil), f.calls...)
}

func (f *fakeGH) count(method, path string) int {
	n := 0
	for _, c := range f.snapshot() {
		if c.Method == method && c.Path == path {
			n++
		}
	}
	return n
}

type fakeClient struct {
	f    *fakeGH
	auth rest.AuthFunc
}

func (c *fakeClient) DoQuery(ctx context.Context, method, path string, params map[string]string, q url.Values, body, out any) (rest.Response, error) {
	f := c.f
	bearer, err := c.auth(ctx)
	if err != nil {
		return rest.Response{}, err
	}
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	f.mu.Lock()
	f.calls = append(f.calls, call{Method: method, Path: path, Params: params, Query: q, Body: raw, Bearer: bearer})
	f.mu.Unlock()
	if f.failPath == path {
		return rest.Response{Status: 500}, f.failErr
	}
	var reply string
	switch {
	case method == "POST" && path == "/app/installations/{id}/access_tokens":
		f.verifyJWT(bearer)
		b := fixture(f.t, "token-reply.json")
		if f.expires != "" {
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			m["expires_at"] = f.expires
			b, _ = json.Marshal(m)
		}
		return rest.Response{Status: 201}, json.Unmarshal(b, out)
	case method == "GET":
		reply = f.list
	case method == "POST":
		reply = "create-reply.json"
	default:
		return rest.Response{Status: 200}, nil
	}
	if bearer != testToken {
		f.t.Errorf("%s %s used a bearer that is not the installation token", method, path)
	}
	return rest.Response{Status: 200}, json.Unmarshal(fixture(f.t, reply), out)
}

// verifyJWT checks the token exchange is signed RS256 by the App key with sane claims.
func (f *fakeGH) verifyJWT(jwt string) {
	f.t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		f.t.Errorf("token exchange did not carry a JWT")
		return
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err != nil || rsa.VerifyPKCS1v15(f.pub, crypto.SHA256, sum[:], sig) != nil {
		f.t.Errorf("the JWT signature does not verify")
	}
	var claims struct {
		Iss      string `json:"iss"`
		Iat, Exp int64
	}
	b, _ := base64.RawURLEncoding.DecodeString(parts[1])
	_ = json.Unmarshal(b, &claims)
	if claims.Iss != "12345" || claims.Exp-claims.Iat > 600 || claims.Exp <= claims.Iat {
		f.t.Errorf("JWT claims: %+v", claims)
	}
}

var errBoom = errors.New("boom")

// fixtureEvidence is a run with one passing check, one code failure (with located excerpt lines)
// and one skipped check.
func fixtureEvidence(results ...model.Check) model.Evidence {
	return model.Evidence{RunID: "run-0001", Revision: testSHA, Checks: results}
}

func check(id string, result model.CheckResult, mut func(*model.Check)) model.Check {
	c := model.Check{ID: id, JobID: id, Kind: "test", Tool: "go", Required: true, Substantive: true, Result: result}
	if mut != nil {
		mut(&c)
	}
	return c
}

func status(state report.State) report.Status {
	return report.Status{Owner: "acme", Repo: "widgets", Revision: testSHA, State: state, Description: "d"}
}

func request(state report.State, ev model.Evidence) Request {
	ev.Result = map[report.State]model.Result{report.Success: "pass", report.Failure: "fail", report.Error: "error"}[state]
	return Request{Owner: "acme", Repo: "widgets", RunID: "run-0001", Revision: testSHA, Status: status(state), Evidence: ev, Projection: model.Projection(ev)}
}
