package run

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/checks"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"

	"encoding/pem"
)

// fakeFunnel stands in for the REST funnel: it records every request and answers like GitHub
// (recorded shapes: an installation token, an empty check-run list, a created check run).
type fakeFunnel struct {
	mu    sync.Mutex
	calls []funnelCall
	fail  bool
}

type funnelCall struct {
	Method, Path, Bearer string
	Body                 []byte
}

func (f *fakeFunnel) factory(_ appSettings) checks.ClientFactory {
	return func(auth rest.AuthFunc) (checks.API, error) { return funnelClient{f, auth}, nil }
}

func (f *fakeFunnel) snapshot() []funnelCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]funnelCall(nil), f.calls...)
}

func (f *fakeFunnel) checkRuns() (n int, last funnelCall) {
	for _, c := range f.snapshot() {
		if c.Method == "POST" && strings.HasSuffix(c.Path, "/check-runs") {
			n, last = n+1, c
		}
	}
	return
}

type funnelClient struct {
	f    *fakeFunnel
	auth rest.AuthFunc
}

func (c funnelClient) DoQuery(ctx context.Context, method, path string, _ map[string]string, _ url.Values, body, out any) (rest.Response, error) {
	bearer, err := c.auth(ctx)
	if err != nil {
		return rest.Response{}, err
	}
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	c.f.mu.Lock()
	c.f.calls = append(c.f.calls, funnelCall{method, path, bearer, raw})
	fail := c.f.fail
	c.f.mu.Unlock()
	if fail && strings.HasSuffix(path, "/check-runs") {
		return rest.Response{Status: 500}, errors.New("server error for Bearer " + bearer)
	}
	reply := `{}`
	switch {
	case strings.HasSuffix(path, "/access_tokens"):
		reply = `{"token":"fixture-installation-token-0002","expires_at":"2099-01-01T00:00:00Z"}`
	case method == "GET":
		reply = `{"total_count":0,"check_runs":[]}`
	case method == "POST":
		reply = `{"id":4242,"name":"nself-ci/gates"}`
	}
	if out != nil {
		_ = json.Unmarshal([]byte(reply), out)
	}
	return rest.Response{Status: 200}, nil
}

func appPEM(t *testing.T) string {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}))
}

func appDetail(t *testing.T, f *fakeFunnel, s appSettings, key string) *checksDetail {
	return &checksDetail{
		Load:    func(context.Context) (appSettings, error) { return s, nil },
		Getenv:  func(name string) string { return map[string]string{"NSELF_CI_APP_KEY_VAR": key}[name] },
		Client:  f.factory,
		Now:     func() time.Time { return time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC) },
		Enabled: func() bool { return true },
	}
}

var enabledApp = appSettings{App: checks.Config{AppID: "12345", InstallationID: "777", KeyEnv: "NSELF_CI_APP_KEY_VAR"}, APIURL: "https://api.github.com"}

// TestDetailPublisherTwoOutputs: through the real run path a failing run posts the status AND
// the check run (two outputs, same conclusion); a passing run concludes success.
func TestDetailPublisherTwoOutputs(t *testing.T) {
	f := &fakeFunnel{}
	r := failRig(t)
	r.C.Hooks.Auto.Details = []PublishDetail{appDetail(t, f, enabledApp, appPEM(t))}
	res := r.run(t)
	posted := r.Rep.all()
	if len(posted) != 1 || posted[0].State != report.Failure {
		t.Fatalf("status of record: %+v", posted)
	}
	n, last := f.checkRuns()
	if n != 1 {
		t.Fatalf("want one check run, calls %+v", f.snapshot())
	}
	var body struct {
		Name       string `json:"name"`
		HeadSHA    string `json:"head_sha"`
		Conclusion string `json:"conclusion"`
		ExternalID string `json:"external_id"`
	}
	_ = json.Unmarshal(last.Body, &body)
	if body.Name != "nself-ci/gates" || body.Conclusion != "failure" || body.HeadSHA != posted[0].Revision || body.ExternalID != res.RunID || last.Bearer != "fixture-installation-token-0002" {
		t.Fatalf("check run: %+v bearer %q", body, last.Bearer)
	}
	for _, note := range res.Summary.Notes {
		if strings.Contains(note, "publish detail") {
			t.Fatalf("unexpected note: %s", note)
		}
	}

	// a passing run
	f = &fakeFunnel{}
	r = passRig(t)
	r.C.Hooks.Auto.Details = []PublishDetail{appDetail(t, f, enabledApp, appPEM(t))}
	r.run(t)
	_, last = f.checkRuns()
	_ = json.Unmarshal(last.Body, &body)
	if p := r.Rep.all(); len(p) != 1 || p[0].State != report.Success || body.Conclusion != "success" {
		t.Fatalf("pass: posted %+v conclusion %q", p, body.Conclusion)
	}

	// removing the App key leaves only the status
	f = &fakeFunnel{}
	r = failRig(t)
	r.C.Hooks.Auto.Details = []PublishDetail{appDetail(t, f, enabledApp, "")}
	r.run(t)
	if p := r.Rep.all(); len(p) != 1 || p[0].State != report.Failure || len(f.snapshot()) != 0 {
		t.Fatalf("no key: posted %+v calls %+v", p, f.snapshot())
	}
}

// TestChecksDisabledWithoutApp: with no App configured every command behaves as before and no
// check-run (or any GitHub) call is made; a half-set App is a note and changes nothing else.
func TestChecksDisabledWithoutApp(t *testing.T) {
	baseline := failRig(t).run(t)

	f := &fakeFunnel{}
	r := failRig(t)
	r.C.Hooks.Auto.Details = []PublishDetail{appDetail(t, f, appSettings{}, appPEM(t))}
	res := r.run(t)
	if len(f.snapshot()) != 0 || res.Exit != baseline.Exit || len(r.Rep.all()) != 1 || len(res.Summary.Notes) != len(baseline.Summary.Notes) {
		t.Fatalf("no App: calls %+v exit %d/%d notes %v", f.snapshot(), res.Exit, baseline.Exit, res.Summary.Notes)
	}

	// v1.4 behaviour (compat off): inert even with everything set
	f = &fakeFunnel{}
	d := appDetail(t, f, enabledApp, appPEM(t))
	d.Enabled = func() bool { return false }
	r = failRig(t)
	r.C.Hooks.Auto.Details = []PublishDetail{d}
	r.run(t)
	if len(f.snapshot()) != 0 {
		t.Fatalf("v1.4 mode made calls: %+v", f.snapshot())
	}

	// partial configuration: a note, no call, same verdict and status
	f = &fakeFunnel{}
	r = failRig(t)
	r.C.Hooks.Auto.Details = []PublishDetail{appDetail(t, f, appSettings{App: checks.Config{AppID: "1"}}, appPEM(t))}
	res = r.run(t)
	found := false
	for _, n := range res.Summary.Notes {
		found = found || strings.HasPrefix(n, "publish detail: E816")
	}
	if !found || len(f.snapshot()) != 0 || res.Exit != baseline.Exit || len(r.Rep.all()) != 1 {
		t.Fatalf("partial: notes %v calls %+v", res.Summary.Notes, f.snapshot())
	}

	// unreadable policy: a note, no call
	d = appDetail(t, f, enabledApp, appPEM(t))
	d.Load = func(context.Context) (appSettings, error) { return appSettings{}, errors.New("db locked") }
	if err := d.Detail(context.Background(), PublishDetailRequest{}); err == nil || len(f.snapshot()) != 0 {
		t.Fatalf("err %v", err)
	}
}

// TestChecksFailureIsANote: a GitHub failure is a note carrying E816; the status, the verdict and
// the exit code are the same, and the token the server echoed is masked.
func TestChecksFailureIsANote(t *testing.T) {
	baseline := failRig(t).run(t)
	f := &fakeFunnel{fail: true}
	r := failRig(t)
	r.C.Hooks.Auto.Details = []PublishDetail{appDetail(t, f, enabledApp, appPEM(t))}
	res := r.run(t)
	var note string
	for _, n := range res.Summary.Notes {
		if strings.HasPrefix(n, "publish detail:") {
			note = n
		}
	}
	if !strings.Contains(note, "E816") || strings.Contains(note, "fixture-installation-token") || res.Exit != baseline.Exit || len(r.Rep.all()) != 1 {
		t.Fatalf("note %q exit %d/%d posted %+v", note, res.Exit, baseline.Exit, r.Rep.all())
	}
}

// TestChecksPublisherCached: the installation token is exchanged once across runs of one process
// and again after the key changes.
func TestChecksPublisherCached(t *testing.T) {
	f := &fakeFunnel{}
	key := appPEM(t)
	d := appDetail(t, f, enabledApp, key)
	req := PublishDetailRequest{Owner: "acme", Repo: "w", RunID: "r1", Revision: strings.Repeat("ab", 20),
		Status: report.Status{Owner: "acme", Repo: "w", Revision: strings.Repeat("ab", 20), State: report.Success, Description: "d"}, Evidence: model.Evidence{Result: "pass"}}
	for i := 0; i < 2; i++ {
		if err := d.Detail(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	exchanges := func() (n int) {
		for _, c := range f.snapshot() {
			if strings.HasSuffix(c.Path, "/access_tokens") {
				n++
			}
		}
		return
	}
	if exchanges() != 1 {
		t.Fatalf("exchanges %d", exchanges())
	}
	d.Getenv = func(string) string { return appPEM(t) }
	if err := d.Detail(context.Background(), req); err != nil || exchanges() != 2 {
		t.Fatalf("a new key must rebuild the publisher: %v exchanges %d", err, exchanges())
	}
}

func TestKeyText(t *testing.T) {
	if keyText("  ") != "" || keyText("a\\nb") != "a\nb" || keyText("a\nb\\nc") != "a\nb\\nc" {
		t.Fatal("keyText")
	}
}

// TestLoadAppSettingsReadsOperatorRows: the production loader finds the three App keys and the API URL
// stored as operator rows through the real resolver; with no rows it reports no App.
func TestLoadAppSettingsReadsOperatorRows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	got, err := loadAppSettings(context.Background())
	if err != nil || got.App.Enabled() || got.App.Partial() {
		t.Fatalf("empty store: %+v %v", got, err)
	}
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{checks.KeyAppID: `12345`, checks.KeyInstallationID: `777`, checks.KeyKeyEnv: `"NSELF_CI_APP_KEY_VAR"`, runtime.KeyGitHubAPIURL: `"https://ghe.example/api/v3"`} {
		if err = st.ApplyWrite(context.Background(), store.PolicyWrite{Op: store.PolicySet, Scope: "user", Subject: policy.LocalSubject, Key: k, Value: []byte(v), Actor: "test"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.Close()
	got, err = loadAppSettings(context.Background())
	if err != nil || got.App != (checks.Config{AppID: "12345", InstallationID: "777", KeyEnv: "NSELF_CI_APP_KEY_VAR"}) || got.APIURL != "https://ghe.example/api/v3" {
		t.Fatalf("operator rows: %+v %v", got, err)
	}
}

// TestChecksConclusionFollowsRunResult: through the real evidence rule (runResult) and the real status
// rule (defaultStatus), the check run's conclusion agrees with the status for every shape of run.
func TestChecksConclusionFollowsRunResult(t *testing.T) {
	chk := func(id string, result model.CheckResult, mut func(*model.Check)) model.Check {
		c := model.Check{ID: id, JobID: id, Kind: "test", Required: true, Substantive: true, Result: result}
		if mut != nil {
			mut(&c)
		}
		return c
	}
	cases := []struct {
		name   string
		checks []model.Check
		state  report.State
		want   checks.Conclusion
	}{
		{"all pass", []model.Check{chk("a", "pass", nil)}, report.Success, checks.Success},
		{"code failure", []model.Check{chk("a", "pass", nil), chk("b", "fail", nil)}, report.Failure, checks.Failure},
		{"infra error only", []model.Check{chk("a", "error", func(c *model.Check) { c.FailureClass = "infra" })}, report.Error, checks.Neutral},
		{"lost runner", []model.Check{chk("a", "error", func(c *model.Check) { c.FailureClass = "lost" })}, report.Error, checks.Neutral},
		{"--job: unselected required job skipped, selected one errors", []model.Check{chk("a", "skip", func(c *model.Check) { c.Reason = "pipeline.excluded" }), chk("b", "error", nil)}, report.Failure, checks.Failure},
		{"error plus code failure", []model.Check{chk("a", "error", nil), chk("b", "fail", nil)}, report.Failure, checks.Failure},
	}
	for _, c := range cases {
		ev := model.Evidence{Checks: c.checks}
		ev.Result = runResult(c.checks)
		st := defaultStatus(PublishRequest{Run: RunInfo{Result: ev.Result}, Projection: model.Projection(ev)})
		if st.State != c.state {
			t.Fatalf("%s: status %s, want %s (the rig is wrong)", c.name, st.State, c.state)
		}
		if got := checks.ConclusionFor(st.State, ev); got != c.want {
			t.Errorf("%s: conclusion %q, want %q", c.name, got, c.want)
		}
	}
}
