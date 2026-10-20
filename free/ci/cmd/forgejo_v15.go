package main

// Purpose: `nself ci forgejo` in v1.5 (P7-HOST-12, Epic D14): the V15-only entry beside the frozen v1.4
// one (cmd/forgejo.go). It prints one deprecation line per process, then runs the Forgejo check of
// internal/forgejo (server health and the optional admin API; no runner container, P7-TRUST-21
// removed that runner). Removal is v1.6 (D-0119); `nself ci doctor --providers` is the way forward.
// Inputs:  --url (default: the operator policy key providers.forgejo.url, else http://localhost:3844),
//          --json; NSELF_FORGEJO_ADMIN_USER and NSELF_FORGEJO_ADMIN_PASSWORD for the API probe.
// Outputs: the deprecation line on stderr, the status lines (or one v1 JSON envelope) on stdout;
//          exit 0 healthy, 1 the server is not healthy, 2 usage.
// Constraints: V15 only, v1.4 argv and output untouched; the password is read from the environment
//          and never printed; a policy that cannot be read falls back to the default URL with a
//          warning (the alias must keep working where nothing is configured).

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/forgejo"
)

var forgejoDeprecationOnce sync.Once

type forgejoV15Flags struct {
	url  string
	json bool
}

func newForgejoV15Flags(f *forgejoV15Flags) *flag.FlagSet {
	fs := flag.NewFlagSet("forgejo", flag.ContinueOnError)
	fs.StringVar(&f.url, "url", "", "Forgejo base URL (default: policy key "+forgejo.KeyURL+", else "+forgejo.DefaultURL+")")
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	return fs
}

// forgejoV15Env is what the command reads from its process; tests replace the fields.
type forgejoV15Env struct {
	Out, Err io.Writer
	Getenv   func(string) string
	// PolicyURL returns the policy key's value, forgejo.DefaultURL when unset.
	PolicyURL func(ctx context.Context) (string, error)
	Client    *http.Client
}

func productionForgejoEnv() forgejoV15Env {
	return forgejoV15Env{Out: os.Stdout, Err: os.Stderr, Getenv: os.Getenv, PolicyURL: policyForgejoURL}
}

func policyForgejoURL(ctx context.Context) (string, error) {
	root, err := evidenceRoot("")
	if err != nil {
		return "", err
	}
	cfg, closeFn := loadProviderConfig(ctx, root)
	defer closeFn()
	if cfg.Effective != nil && cfg.ForgejoURL != "" {
		return cfg.ForgejoURL, nil // an unrelated bad providers.* value must not hide a valid Forgejo URL
	}
	return cfg.ForgejoURL, cfg.Err
}

func init() {
	registerV15("forgejo", subcommandEntry{V15Only: true,
		Flags:   func() *flag.FlagSet { return newForgejoV15Flags(&forgejoV15Flags{}) },
		Handler: func(args []string) int { return runForgejoV15(args, productionForgejoEnv()) }})
}

func runForgejoV15(args []string, env forgejoV15Env) int {
	var f forgejoV15Flags
	fs := newForgejoV15Flags(&f)
	fs.SetOutput(env.Err)
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(env.Err, "usage: nself ci forgejo [--url URL] [--json]")
		return 2
	}
	forgejoDeprecationOnce.Do(func() { _, _ = fmt.Fprintln(env.Err, forgejo.Deprecation) })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	base := f.url
	if base == "" {
		var err error
		if base, err = env.PolicyURL(ctx); err != nil || base == "" {
			_, _ = fmt.Fprintf(env.Err, "warning: the policy could not give %s (%v); using %s\n", forgejo.KeyURL, err, forgejo.DefaultURL)
			base = forgejo.DefaultURL
		}
	}
	res := forgejo.Check(ctx, forgejo.CheckConfig{URL: base, User: env.Getenv("NSELF_FORGEJO_ADMIN_USER"), Pass: env.Getenv("NSELF_FORGEJO_ADMIN_PASSWORD"), Client: env.Client})
	if f.json {
		if output.WriteData(env.Out, "ci forgejo", res, &output.Meta{Warnings: []string{forgejo.Deprecation}}) != nil {
			return 2
		}
	} else {
		forgejo.Print(env.Out, res)
	}
	if !res.Healthy {
		return 1
	}
	return 0
}
