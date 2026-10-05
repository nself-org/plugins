package main

// Purpose: the "serve" subcommand: self-hosted CI webhook listener daemon,
// ported from core `nself ci serve` (P7-CANON-09). The implementation lives in
// internal/serve; this file parses the flags and the environment merge.
// Inputs:  --addr, --secret, --concurrency, --workdir, --timeout, -v/--verbose,
// --insecure, --allow-unsandboxed, --allowed-repos (comma list, repeatable);
// env GITHUB_WEBHOOK_SECRET, NSELF_CI_ALLOWED_REPOS, NSELF_CI_ALLOW_UNSANDBOXED.
// Outputs: HTTP server on :3845 until SIGINT/SIGTERM; exit 0, or 1 on error.
// Constraints: the fail-closed defaults (secret, allowlist, Docker) are
// unchanged; the gate binary a job runs is this executable (os.Executable).

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/serve"
)

func init() {
	register("serve", func(args []string) int { return serveCmd(args, flag.ExitOnError) })
}

// listFlag collects a comma-separated, repeatable flag (cobra StringSlice).
type listFlag struct{ vals []string }

func (l *listFlag) String() string { return strings.Join(l.vals, ",") }

// Set appends the comma-separated values of one occurrence.
func (l *listFlag) Set(v string) error {
	if v == "" {
		return nil
	}
	for _, p := range strings.Split(v, ",") {
		l.vals = append(l.vals, p)
	}
	return nil
}

// parseServeArgs builds the serve config from argv and the environment.
func parseServeArgs(args []string, eh flag.ErrorHandling) (serve.ServeConfig, error) {
	var cfg serve.ServeConfig
	var allowed listFlag
	fs := flag.NewFlagSet("serve", eh)
	fs.StringVar(&cfg.Addr, "addr", ":3845", "Listen address (host:port). Port 3845 = nself-ci-serve per F10 port registry")
	fs.StringVar(&cfg.Secret, "secret", "", "HMAC-SHA256 webhook secret (overrides GITHUB_WEBHOOK_SECRET env)")
	fs.IntVar(&cfg.Concurrency, "concurrency", 2, "Max concurrent CI jobs")
	fs.StringVar(&cfg.WorkDir, "workdir", "/tmp/nself-ci-workdirs", "Base directory for ephemeral checkout workdirs")
	fs.IntVar(&cfg.JobTimeout, "timeout", 600, "Per-job timeout in seconds")
	fs.BoolVar(&cfg.Verbose, "v", false, "Verbose gate output")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "Alias for -v (core spelling)")
	fs.BoolVar(&cfg.Insecure, "insecure", false, "DANGEROUS: start without a webhook secret (disables signature verification).")
	fs.BoolVar(&cfg.AllowUnsandboxed, "allow-unsandboxed", false, "DANGEROUS: run the gate directly on this host when Docker is unavailable, instead of refusing the job.")
	fs.Var(&allowed, "allowed-repos", "Comma-separated or repeatable \"owner/repo\" allowlist.")
	if _, err := parseInterspersed(fs, args); err != nil {
		return cfg, err
	}
	if !cfg.AllowUnsandboxed && strings.EqualFold(os.Getenv("NSELF_CI_ALLOW_UNSANDBOXED"), "true") {
		cfg.AllowUnsandboxed = true
	}
	cfg.AllowedRepos = append(allowed.vals, splitAllowedReposEnv(os.Getenv("NSELF_CI_ALLOWED_REPOS"))...)
	return cfg, nil
}

// serveCmd parses the flags and runs the daemon.
func serveCmd(args []string, eh flag.ErrorHandling) int {
	cfg, err := parseServeArgs(args, eh)
	if err != nil {
		return 2
	}
	if err := serve.RunServe(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// splitAllowedReposEnv parses a comma-separated NSELF_CI_ALLOWED_REPOS value
// into a slice, trimming whitespace and dropping empty entries.
func splitAllowedReposEnv(v string) []string {
	if v == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
