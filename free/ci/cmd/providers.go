package main

// Purpose: `nself ci providers list|show|test|authorize|revoke` (P7-HOST-12, Epic D14, X18): the group
// help, the shared flags and environment of the five commands, and `list`. `show` is in
// providers_show.go, `test` in providers_probe*.go, `authorize` and `revoke` in
// providers_authorize.go; `github init`, `gitlab init` and `usage import` are the earlier Tickets'.
// Inputs:  argv after the key; env NSELF_CI_HOME (state.db), NSELF_CI_REPO (repository root),
//          the token variables the operator policy names.
// Outputs: a table or one v1 JSON envelope (--json); exit 0 ok, 1 refused or a failed check, 2
//          infrastructure.
// Constraints: V15 only; side-effect classes: list, show and test read (test is online), authorize and
//          revoke write the operator policy through the P7-TRUST-26 writer and nothing else; there is
//          no doctor subcommand (provider checks are `nself ci doctor --providers`); no command
//          prints a credential value, only its source, presence and kind.

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	cidoc "github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	pdoctor "github.com/nself-org/plugins/free/ci/internal/providers/doctor"
	"github.com/nself-org/plugins/free/ci/internal/providers/health"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const providersHelp = `nself ci providers <command> ...  (hosted capacity: GitHub and GitLab)

  list [--json]                                       the providers, their health and credentials
  show <id> [--repo owner/name] [--json]              capabilities, health, economics and stored facts
  test <id> [--repo owner/name] [--json]              online: credential, scopes, repo access, workflow, secrets
  authorize <id> --project <p> [--allow-private] [--json]
  revoke <id> --project <p> [--json]
  github init [--check|--stdout] ...                  write or check the dispatch workflow
  gitlab init ...                                     write the pipeline include
  usage import ...                                    import provider-reported usage

Provider checks are a section of the doctor: nself ci doctor --providers
`

// providersFlags holds every flag of the five commands; each registers its own subset.
type providersFlags struct {
	json, allowPrivate bool
	project, repo      string
}

func providersFlagSet(op string, f *providersFlags, w io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("providers "+op, flag.ContinueOnError)
	fs.SetOutput(w)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	switch op {
	case "show", "test":
		fs.StringVar(&f.repo, "repo", "", "repository: owner/name (GitHub) or group/project (GitLab); default: the origin remote")
	case "authorize", "revoke":
		fs.StringVar(&f.project, "project", "", "the project id the policy row applies to (required)")
	}
	if op == "authorize" {
		fs.BoolVar(&f.allowPrivate, "allow-private", false, "also let private repositories use the provider (writes trust.privacy_zone)")
	}
	return fs
}

// providersEnv is the environment of the providers commands; tests replace its fields.
type providersEnv struct {
	policyEnv
	Getenv func(string) string
	Now    func() time.Time
	// OpenStore returns the CI store when one exists, else (nil, nil); it never creates one.
	OpenStore func() (*store.Store, error)
	// Apply writes one policy row; nil means the P7-TRUST-26 writer over the opened store.
	Apply func(ctx context.Context, actor roles.Actor, c policy.Change) (policy.Result, error)
	// NewClient builds the REST client of a provider family for `test` and `show`.
	NewClient func(cfg pdoctor.Config, family string, runner bool) (*rest.Client, error)
}

func productionProvidersEnv() providersEnv {
	return providersEnv{policyEnv: productionPolicyEnv(), Getenv: os.Getenv, Now: time.Now,
		OpenStore: func() (*store.Store, error) { return pdoctor.OpenExisting(os.Getenv) }, NewClient: productionProviderClient}
}

func init() {
	registerV15("providers", subcommandEntry{V15Only: true,
		Flags: func() *flag.FlagSet { return flag.NewFlagSet("providers", flag.ContinueOnError) },
		Handler: func(args []string) int {
			fmt.Fprint(os.Stdout, providersHelp)
			if len(args) > 0 && args[0] != "--help" && args[0] != "-h" && args[0] != "help" {
				return 1
			}
			return 0
		}})
	for _, op := range []string{"list", "show", "test", "authorize", "revoke"} {
		op := op
		registerV15("providers "+op, subcommandEntry{V15Only: true,
			Flags:   func() *flag.FlagSet { var f providersFlags; return providersFlagSet(op, &f, io.Discard) },
			Handler: func(args []string) int { return runProviders(op, args, productionProvidersEnv()) }})
	}
}

// loadProviderConfig resolves the provider configuration of the repository at root through the
// existing CI store; closeFn closes the store.
func loadProviderConfig(ctx context.Context, root string) (pdoctor.Config, func()) {
	st, err := pdoctor.OpenExisting(os.Getenv)
	if err != nil {
		st = nil
	}
	closeFn := func() {}
	if st != nil {
		closeFn = func() { _ = st.Close() }
	}
	return pdoctor.Load(ctx, root, report.Capture, st), closeFn
}

func runProviders(op string, args []string, env providersEnv) int {
	command := "ci providers " + op
	var f providersFlags
	pos, err := adminParse(providersFlagSet(op, &f, env.Err), args)
	if err != nil {
		return adminFail(env.adminEnv, command, adminHasJSON(args), err)
	}
	if want := map[bool]int{true: 0, false: 1}[op == "list"]; len(pos) != want {
		return adminFail(env.adminEnv, command, f.json, adminUsage("usage: nself ci providers %s%s", op, map[string]string{
			"list": " [--json]", "show": " <id> [--repo R] [--json]", "test": " <id> [--repo R] [--json]",
			"authorize": " <id> --project <p> [--allow-private] [--json]", "revoke": " <id> --project <p> [--json]"}[op]))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	root, err := env.Root()
	if err != nil {
		return adminFail(env.adminEnv, command, f.json, fmt.Errorf("the repository root is unknown: %w", err))
	}
	if op == "authorize" || op == "revoke" {
		return runAuthorize(ctx, op, pos[0], f, env)
	}
	st, err := env.OpenStore()
	if err != nil {
		return adminFail(env.adminEnv, command, f.json, fmt.Errorf("the CI store cannot be opened: %w", err))
	}
	if st != nil {
		defer func() { _ = st.Close() }()
	}
	cfg := pdoctor.Load(ctx, root, env.Run, st)
	switch op {
	case "list":
		return listProviders(ctx, env, cfg, st, f)
	case "show":
		return showProvider(ctx, env, cfg, st, root, pos[0], f)
	}
	return testProvider(ctx, env, cfg, st, root, pos[0], f)
}

type healthJSON struct {
	State   string `json:"state"`
	Reason  string `json:"reason,omitempty"`
	Since   string `json:"since,omitempty"`
	RetryAt string `json:"retry_at,omitempty"`
}

func healthOf(ctx context.Context, st *store.Store, id string) healthJSON {
	opts := health.Options{}
	if st != nil {
		opts.Store = st
	}
	h := health.New(opts).HealthOf(ctx, id)
	out := healthJSON{State: h.State, Reason: h.Reason}
	if !h.Since.IsZero() {
		out.Since = h.Since.UTC().Format(time.RFC3339)
	}
	if !h.RetryAt.IsZero() {
		out.RetryAt = h.RetryAt.UTC().Format(time.RFC3339)
	}
	return out
}

// credentialOf is the credential row of a provider id (GitHub and GitLab families only).
func credentialOf(ctx context.Context, env providersEnv, cfg pdoctor.Config, id string) *pdoctor.Credential {
	switch health.Family(id) {
	case "github":
		c := pdoctor.GitHubCredential(ctx, cfg, env.Getenv, cidoc.CaptureFunc(env.Run))
		return &c
	case "gitlab":
		c := pdoctor.GitLabCredential(cfg, env.Getenv, false)
		return &c
	}
	return nil
}

type providerRow struct {
	ID         string              `json:"id"`
	Family     string              `json:"family"`
	Health     healthJSON          `json:"health"`
	Credential *pdoctor.Credential `json:"credential,omitempty"`
	Authorized bool                `json:"authorized"`
}

func listProviders(ctx context.Context, env providersEnv, cfg pdoctor.Config, st *store.Store, f providersFlags) int {
	const command = "ci providers list"
	rows := []providerRow{}
	for _, p := range providers.All() {
		rows = append(rows, providerRow{ID: p.ID(), Family: health.Family(p.ID()), Health: healthOf(ctx, st, p.ID()),
			Credential: credentialOf(ctx, env, cfg, p.ID()), Authorized: slices.Contains(cfg.Authorized, p.ID())})
	}
	if f.json {
		return adminWriteData(env.adminEnv, command, map[string]any{"providers": rows, "privacy_zone": cfg.PrivacyZone, "authorized": append([]string{}, cfg.Authorized...)})
	}
	_, _ = fmt.Fprintf(env.Out, "%-16s %-10s %-34s %s\n", "PROVIDER", "HEALTH", "CREDENTIAL", "AUTHORIZED")
	for _, r := range rows {
		cred := "-"
		if r.Credential != nil {
			cred = fmt.Sprintf("%s (%s)", plainText(r.Credential.Source), map[bool]string{true: "set", false: "missing"}[r.Credential.Present])
		}
		_, _ = fmt.Fprintf(env.Out, "%-16s %-10s %-34s %v\n", plainText(r.ID), r.Health.State, cred, r.Authorized)
	}
	return 0
}
