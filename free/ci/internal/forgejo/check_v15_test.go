package forgejo

// Purpose: tests of the v1.5 Forgejo check (P7-HOST-12): the server and admin API probe against an
// httptest server, the policy key's decoder and default, and the rule that the admin password goes only
// into the basic-auth header and never into the result or the printed lines.
// Inputs:  httptest servers on loopback. Outputs: pass or fail; no network beyond loopback.
// Constraints: tests only; fake values only.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
)

func server(t *testing.T, healthy bool, apiStatus int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/-/health":
			_, _ = w.Write([]byte(map[bool]string{true: `{"healthy": true}`, false: `{"healthy": false}`}[healthy]))
		case "/api/v1/repos/search":
			if _, p, ok := r.BasicAuth(); !ok || p != "fake-password-0001" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(apiStatus)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCheckHealthyWithAdminAPI(t *testing.T) {
	srv := server(t, true, 200)
	res := Check(context.Background(), CheckConfig{URL: srv.URL + "/", User: "admin", Pass: "fake-password-0001"})
	if !res.Healthy || res.Server != "healthy" || res.API != "reachable (admin authenticated)" || res.URL != srv.URL {
		t.Fatalf("result: %+v", res)
	}
	var buf bytes.Buffer
	Print(&buf, res)
	if strings.Contains(buf.String(), "fake-password") || strings.Contains(buf.String(), "runner") {
		t.Fatalf("the lines must hold neither the password nor a runner line: %s", buf.String())
	}
	raw, _ := json.Marshal(res)
	if strings.Contains(string(raw), "fake-password") {
		t.Fatalf("the JSON holds the password: %s", raw)
	}
}

func TestCheckStates(t *testing.T) {
	srv := server(t, true, 200)
	if res := Check(context.Background(), CheckConfig{URL: srv.URL}); !res.Healthy || res.API != APINotConfigured {
		t.Fatalf("no credentials: %+v", res)
	}
	if res := Check(context.Background(), CheckConfig{URL: srv.URL, User: "admin", Pass: "wrong"}); res.API != "HTTP 401" {
		t.Fatalf("a rejected password: %+v", res)
	}
	sick := server(t, false, 200)
	res := Check(context.Background(), CheckConfig{URL: sick.URL, User: "admin", Pass: "fake-password-0001"})
	if res.Healthy || res.Server != "HTTP 200" || res.API != APINotConfigured {
		t.Fatalf("an unhealthy server skips the API: %+v", res)
	}
	var buf bytes.Buffer
	Print(&buf, res)
	if !strings.Contains(buf.String(), "not healthy") || !strings.Contains(buf.String(), "✗ Forgejo server") {
		t.Fatalf("unhealthy lines: %s", buf.String())
	}
	if res := Check(context.Background(), CheckConfig{URL: "http://127.0.0.1:1"}); res.Healthy || res.Server != "unreachable" {
		t.Fatalf("unreachable: %+v", res)
	}
	if res := Check(context.Background(), CheckConfig{URL: "ftp://x"}); res.Healthy || res.Server != "invalid URL" {
		t.Fatalf("invalid URL: %+v", res)
	}
}

func TestPolicyKey(t *testing.T) {
	if len(RegistrationErrors()) != 0 {
		t.Fatalf("registration: %v", RegistrationErrors())
	}
	spec, ok := policy.Default.Find(KeyURL)
	if !ok || !spec.Spend || spec.Kind != policy.TightenFixed {
		t.Fatalf("the key must be an operator-scope Spend key: %+v", spec)
	}
	for _, good := range []string{`"http://localhost:3844"`, `"https://forge.example.com/"`} {
		if _, err := spec.Decode(json.RawMessage(good)); err != nil {
			t.Errorf("%s refused: %v", good, err)
		}
	}
	for _, bad := range []string{`"ftp://x"`, `"https://u:p@host"`, `"https://host?x=1"`, `"host"`, `3`, `["a"]`, `""`} {
		if _, err := spec.Decode(json.RawMessage(bad)); err == nil || !strings.HasPrefix(err.Error(), "E672") {
			t.Errorf("%s: want E672, got %v", bad, err)
		}
	}
	if _, err := URLFrom(nil); err == nil {
		t.Error("a nil policy must be an error, not the default")
	}
}
