package run

// Purpose: the PublishDetail slot filled by the optional GitHub App adapter (P7-AUTO-22): after
// the commit status of a run was delivered, publish the `nself-ci/gates` check run next to it.
// This is the only file of the package that imports internal/providers/github/checks.
// Inputs: a PublishDetailRequest. The operator's providers.github.app.* policy keys (and
// providers.github.api_url), resolved from the default state database like the notify hook does;
// the PEM from the environment variable that providers.github.app.key_env names.
// Outputs: a check run on the commit, and one error line (a note in the run summary) when it
// could not be published. The status, the verdict, the exit code and the claim ledger never
// depend on it.
// Constraints: only under compat.V15(); inert (no store read beyond the policy, no network, no
// process) while the App keys are unset or the variable holding the key is empty: then the run
// behaves exactly as before. The PEM, the JWT and the installation token live in memory only.
// Registered from init(), like the other hooks of this package.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	compat "github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/checks"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

func init() { RegisterPublishDetail(&checksDetail{}) }

// appSettings is what the detail publisher needs from the operator policy.
type appSettings struct {
	App          checks.Config
	APIURL       string
	AllowPrivate bool
}

// checksDetail implements PublishDetail. The zero value is production; tests set the seams.
type checksDetail struct {
	// Load resolves the settings (nil = the default state database).
	Load func(ctx context.Context) (appSettings, error)
	// Getenv reads the variable that holds the PEM (nil = os.Getenv).
	Getenv func(string) string
	// Client builds the REST client of an AuthFunc (nil = the REST funnel against APIURL).
	Client func(s appSettings) checks.ClientFactory
	Now    func() time.Time
	// Enabled says whether v1.5 behaviour is on (nil = compat.V15).
	Enabled func() bool

	mu  sync.Mutex
	pub *checks.Publisher // reused while the settings and the key are unchanged: it caches the installation token
	sig string
}

// Detail implements PublishDetail.
func (h *checksDetail) Detail(ctx context.Context, r PublishDetailRequest) error {
	if enabled := h.Enabled; (enabled == nil && !compat.V15()) || (enabled != nil && !enabled()) {
		return nil
	}
	load := h.Load
	if load == nil {
		load = loadAppSettings
	}
	s, err := load(ctx)
	if err != nil {
		return errors.New("app settings not read, no check run: " + firstLineOf(err.Error()))
	}
	if s.App.Partial() {
		return errors.New("E816: providers.github.app.* is incomplete: set id, installation_id and key_env, or none of them")
	}
	if !s.App.Enabled() {
		return nil
	}
	getenv := h.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	pem := keyText(getenv(s.App.KeyEnv))
	if pem == "" { // no key on this machine: statuses only, as before
		return nil
	}
	pub, err := h.publisher(s, pem)
	if err != nil {
		return err
	}
	return pub.Detail(ctx, checks.Request{Owner: r.Owner, Repo: r.Repo, RunID: r.RunID, Revision: r.Revision,
		Status: r.Status, Evidence: r.Evidence, Projection: r.Projection})
}

// keyText returns the PEM with real line breaks: a key kept on one line with literal "\n" is common in env files.
func keyText(v string) string {
	v = strings.TrimSpace(v)
	if v != "" && !strings.Contains(v, "\n") {
		v = strings.ReplaceAll(v, `\n`, "\n")
	}
	return v
}

// publisher returns the cached publisher or builds one. The cache key is a digest, never the key.
func (h *checksDetail) publisher(s appSettings, pem string) (*checks.Publisher, error) {
	sum := sha256.Sum256([]byte(pem))
	sig := strings.Join([]string{s.App.AppID, s.App.InstallationID, s.APIURL, hex.EncodeToString(sum[:])}, "|")
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pub != nil && h.sig == sig {
		return h.pub, nil
	}
	client := h.Client
	if client == nil {
		client = restFactory
	}
	pub, err := checks.New(s.App, []byte(pem), client(s), h.Now)
	if err != nil {
		return nil, err
	}
	h.pub, h.sig = pub, sig
	return pub, nil
}

// restFactory builds clients on the REST funnel against the operator's GitHub API URL.
func restFactory(s appSettings) checks.ClientFactory {
	return func(auth rest.AuthFunc) (checks.API, error) {
		return &rest.Client{BaseURL: s.APIURL, Auth: auth, UserAgent: "nself-ci-github-checks", APIVersion: "2022-11-28",
			Provider: "github", AllowPrivateBase: s.AllowPrivate}, nil
	}
}

// loadAppSettings resolves the operator's keys against the default state database.
func loadAppSettings(ctx context.Context) (appSettings, error) {
	st, err := OpenStore("")
	if err != nil {
		return appSettings{}, err
	}
	defer func() { _ = st.Close() }()
	run := policy.Runner(report.Capture)
	res := policy.Resolver{Store: st, Run: run}
	got, err := res.Resolve(ctx, policy.Repo{Root: ".", Project: policy.ProjectIDGitCached(ctx, run, "."), Visibility: "unknown", Class: "owner"}, policy.Revision{})
	if err != nil {
		return appSettings{}, err
	}
	app, err := checks.ConfigFrom(&got.Effective)
	if err != nil || !app.Enabled() && !app.Partial() {
		return appSettings{App: app}, err
	}
	api, err := runtime.SettingsFrom(&got.Effective)
	return appSettings{App: app, APIURL: api.GitHubAPIURL, AllowPrivate: api.GitHubAllowPrivateBase}, err
}
