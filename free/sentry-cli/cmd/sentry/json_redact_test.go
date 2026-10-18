package main

// json_redact_test.go: the v1.5 envelope never carries stored credentials (D-0926).
//
// Purpose: monitors, status pages and alert channels hold values the caller
//   stored (URLs with userinfo or token parameters, webhook and bot secrets).
//   In v1.5 mode the envelope copy must not contain them; in v1.4 mode the
//   bytes stay as stored.
// Inputs: a fixture API of httptest servers whose values carry synthetic secrets.
// Outputs: test results.
// Constraints: no network; every secret is synthetic and built at run time so
//   no key-shaped literal sits in the source.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/compat/compattest"
)

// leakyAPI serves values that carry credentials. secrets lists every secret.
func leakyAPI(t *testing.T) (*httptest.Server, []string) {
	t.Helper()
	pass := "pw" + strings.Repeat("x7", 4)
	tok := "tk" + strings.Repeat("q9", 6)
	slack := "SL" + strings.Repeat("a1", 10)
	bot := "123456789:" + "Bt" + strings.Repeat("z3_", 8)
	sig := "sg" + strings.Repeat("m5", 5)
	monURL := "https://ops:" + pass + "@api.example.test/health?token=" + tok + "&region=eu"
	hookURL := "https://hooks.slack.com/services/T0000001/B0000001/" + slack
	pageURL := "https://status.example.test/s/public?sig=" + sig + "#access_token=" + tok
	mon := `{"id":"m1","name":"` + monURL + `","url":"` + monURL + `","kind":"http","interval_seconds":60,"status":"up","paused":false,"created_at":"2026-01-02T03:04:05Z"}`
	page := `{"id":"p1","name":"Public","slug":"public","url":"` + pageURL + `","public":true,"created_at":"2026-01-02T03:04:05Z"}`
	mux := http.NewServeMux()
	reply := func(code int, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		}
	}
	mux.HandleFunc("GET /v1/monitors", reply(200, `{"monitors":[`+mon+`]}`))
	mux.HandleFunc("POST /v1/monitors", reply(201, `{"monitor":`+mon+`}`))
	mux.HandleFunc("POST /v1/monitors/{id}/pause", reply(200, `{"monitor":`+mon+`}`))
	mux.HandleFunc("POST /v1/monitors/{id}/resume", reply(200, `{"monitor":`+mon+`}`))
	mux.HandleFunc("GET /v1/status-pages", reply(200, `{"status_pages":[`+page+`]}`))
	mux.HandleFunc("POST /v1/status-pages", reply(201, `{"status_page":`+page+`}`))
	mux.HandleFunc("GET /v1/alerts/channels", reply(200, `{"channels":[`+
		`{"id":"c1","kind":"slack","target":"`+hookURL+`","enabled":true},`+
		`{"id":"c2","kind":"telegram","target":"`+bot+`","enabled":true},`+
		`{"id":"c3","kind":"webhook","target":"`+monURL+`","enabled":false},`+
		`{"id":"c4","kind":"email","target":"ops@example.test","enabled":true}]}`))
	mux.HandleFunc("POST /v1/alerts/channels/{id}/test", reply(200, `{"delivered":true,"detail":"posted to `+hookURL+`"}`))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, []string{pass, tok, slack, bot, sig}
}

func TestJSONRedactsCredentialValues(t *testing.T) {
	srv, secrets := leakyAPI(t)
	api := []string{"--api-url", srv.URL, "--api-key", fixtureKey, "--json"}
	with := func(a ...string) []string { return append(append([]string{}, a...), api...) }
	cases := []struct {
		name string
		args []string
		keep []string // secrets the v1.4 bytes print unchanged
	}{
		{"monitors-list", with("monitors", "list"), secrets[:2]},
		{"monitors-add", with("monitors", "add", "--url", "https://api.example.test"), secrets[:2]},
		{"monitors-pause", with("monitors", "pause", "m1"), secrets[:2]},
		{"monitors-resume", with("monitors", "resume", "m1"), secrets[:2]},
		{"status-pages-list", with("status-pages", "list"), secrets[4:]},
		{"status-pages-create", with("status-pages", "create", "--name", "Public", "--slug", "public"), secrets[4:]},
		{"alerts-channels", with("alerts", "channels"), secrets[2:4]},
		{"alerts-test", with("alerts", "test", "c1"), secrets[2:3]},
	}
	compattest.Both(t, func(t *testing.T) {
		v15 := strings.HasSuffix(t.Name(), "v1.5")
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				isolate(t)
				r := captureRun(t, c.args)
				if r.code != 0 {
					t.Fatalf("exit = %d (stdout=%s stderr=%s)", r.code, r.stdout, r.stderr)
				}
				if v15 {
					for _, s := range secrets {
						if strings.Contains(r.stdout, s) {
							t.Errorf("v1.5 envelope leaks %q:\n%s", s, r.stdout)
						}
					}
					if !strings.Contains(r.stdout, redactedMark) {
						t.Errorf("v1.5 envelope shows no %s marker:\n%s", redactedMark, r.stdout)
					}
					return
				}
				for _, s := range c.keep {
					// v1.4 prints the bare payload on stderr.
					if !strings.Contains(r.stdout+r.stderr, s) {
						t.Errorf("v1.4 bytes must keep the stored value %q:\n%s", s, r.stdout+r.stderr)
					}
				}
			})
		}
	})
}

func TestRedactValue(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"plain host", "api.example.test", "api.example.test"},
		{"host port", "db.example.test:5432", "db.example.test:5432"},
		{"plain url", "https://api.example.test/health?region=eu", "https://api.example.test/health?region=eu"},
		{"userinfo", "https://u:p@api.example.test/x", "https://REDACTED@api.example.test/x"},
		{"user only", "https://tok@api.example.test/x", "https://REDACTED@api.example.test/x"},
		{"bare userinfo", "u:p@db.example.test:5432", "REDACTED@db.example.test:5432"},
		{"query token", "https://a.test/h?token=abc&region=eu", "https://a.test/h?token=REDACTED&region=eu"},
		{"query api_key", "https://a.test/h?x=1&API_KEY=abc", "https://a.test/h?x=1&API_KEY=REDACTED"},
		{"query escaped name", "https://a.test/h?access%5Ftoken=abc", "https://a.test/h?access%5Ftoken=REDACTED"},
		{"fragment", "https://a.test/h#access_token=abc", "https://a.test/h#REDACTED"},
		{"slack", "https://hooks.slack.com/services/T01/B02/abcDEF", "https://hooks.slack.com/services/T01/REDACTED/REDACTED"},
		{"discord", "https://discord.com/api/webhooks/123/abcDEF", "https://discord.com/api/webhooks/123/REDACTED"},
		{"telegram url", "https://api.telegram.org/bot123456789:" + strings.Repeat("Ab1_", 8) + "/sendMessage", "https://api.telegram.org/REDACTED/sendMessage"},
		{"opaque segment", "https://a.test/hook/" + strings.Repeat("a1", 12), "https://a.test/hook/REDACTED"},
		{"short path kept", "https://status.example.test/s/public", "https://status.example.test/s/public"},
		{"bot token", "123456789:" + strings.Repeat("Ab1_", 8), "REDACTED"},
		{"chat id kept", "-1001234567890", "-1001234567890"},
		{"url inside text", "see https://u:p@a.test/x?token=t ok", "see https://REDACTED@a.test/x?token=REDACTED ok"},
		{"unparsable", "https://u:p@a.test:bad/x?token=t", "https://REDACTED@a.test:bad/x?REDACTED"},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactValue(tc.in); got != tc.want {
				t.Fatalf("redactValue(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRedactTargetKeepsEmail(t *testing.T) {
	if got := redactTarget("email", "ops@example.test"); got != "ops@example.test" {
		t.Fatalf("email target changed: %q", got)
	}
}
