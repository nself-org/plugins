// Package reconcile is the GitHub reconciliation sweep and the poll-mode runner demand poll of the hosted
// providers (P7-HOST-17, Epic D9 and D19). It implements the coordinator's Reconciler twice: GitHub
// finds triggers a webhook missed (redelivering failed hook deliveries when the token may list them,
// otherwise scanning heads that have no terminal nself-ci status), and Demand turns queued
// self-hosted jobs into runner_demand triggers without a webhook. It starts no server and opens no
// listener.
package reconcile

// Purpose: the shared engine behind both Reconcilers: injected clock, store, policy and API client; the
// repository list; the per-token budget ledger; the quota backoff.
// Inputs:  the operator policy (UsePolicy), the coordinator's allowlist and the store (Use), the REST
//          funnel under the operator's GitHub settings.
// Outputs: Engine, with GitHub() and Demand() as the two Reconcilers; Default is the process-wide one the
//          init() registered.
// Constraints: dormant unless compat.V15(); every GET goes through the provider funnel; a pass makes at
//          most MaxPassCalls requests per repository; a quota error stops the pass and records RetryAt, and
//          no request is made before it; the budget ledger keeps 40% of the hourly allowance for demand
//          polling; no test touches the network.

import (
	"context"
	"encoding/json"
	"flag"
	"math/rand/v2"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/webhook"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
)

// Source is the trigger source of every delivery this package enqueues (the same "github" the webhook
// source uses, so the intake's dedup and RefetchFacts treat both alike).
const Source = webhook.Name

// Bounds of one pass.
const (
	MaxPassCalls       = 200  // requests per repository per head-scan pass
	MaxPRs             = 100  // open pull request heads scanned per repository
	MaxBranchHeads     = 20   // default plus protected branch heads scanned per repository
	DefaultHourlyCalls = 4000 // non-304 requests per token per hour (80% of the primary limit)
)

// State is what the Reconcilers keep between passes. *store.Store satisfies it (host-provider set).
type State interface {
	ProviderMarkGet(ctx context.Context, name, key string) (time.Time, bool, error)
	ProviderMarkPut(ctx context.Context, name, key string, at time.Time) error
	ProviderFactGet(ctx context.Context, provider, scope, key string) (StoredFact, bool, error)
	ProviderFactPut(ctx context.Context, f StoredFact) (bool, error)
}

// Engine holds everything the two Reconcilers share. The zero value is not usable: call NewEngine.
type Engine struct {
	Now          func() time.Time
	Jitter       func() float64                                  // in [0,1); the backoff scales it to +-20%
	NewAPI       func(ctx context.Context) (*rest.Client, error) // the REST client (default: operator settings)
	Repos        func(ctx context.Context) ([]string, error)     // the repositories to sweep (default: allowlist)
	HourlyBudget int                                             // non-304 requests per token per hour
	PassCalls    int                                             // requests per repository per head-scan pass (default MaxPassCalls)

	mu     sync.Mutex
	qmu    sync.Mutex // serialises the read-modify-write of the quota record (both Reconcilers write it)
	fmu    sync.Mutex // serialises ledger writes so an older snapshot never lands last
	floor  time.Time  // the latest RetryAt recorded in this process, kept even when the store write fails
	state  State
	ledger ledger
	ticks  map[string]int // per repository, demand ticks since the last in_progress sweep
}

// NewEngine returns an Engine wired to the process: real clock, operator settings and allowlist.
func NewEngine() *Engine {
	return &Engine{Now: time.Now, Jitter: rand.Float64, NewAPI: defaultAPI, Repos: defaultRepos, HourlyBudget: DefaultHourlyCalls, ticks: map[string]int{}}
}

// Use installs the store the marks, ETags and counters live in (the coordinator start does this once).
func (e *Engine) Use(s State) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.state = s
}

func (e *Engine) store() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state
}

func (e *Engine) now() time.Time { return e.Now().UTC() }

func (e *Engine) passLimit() int {
	if e.PassCalls > 0 {
		return min(e.PassCalls, MaxPassCalls)
	}
	return MaxPassCalls
}

// settings reads the effective settings; with no policy installed the defaults apply.
func (e *Engine) settings(ctx context.Context) (Settings, error) {
	eff, err := currentPolicy(ctx)
	if err != nil {
		return Settings{}, err
	}
	if eff == nil {
		return DefaultSettings(), nil
	}
	return SettingsFrom(eff)
}

// defaultAPI builds the client from the operator's settings, as the webhook source does.
func defaultAPI(ctx context.Context) (*rest.Client, error) {
	st, err := runtime.CurrentSettings(ctx)
	if err != nil {
		return nil, err
	}
	return &rest.Client{
		BaseURL: st.GitHubAPIURL, Auth: runtime.Credential(ctx, webhook.Name), Provider: webhook.Name,
		UserAgent: "ci-reconcile", APIVersion: "2022-11-28", AllowPrivateBase: st.GitHubAllowPrivateBase,
	}, nil
}

var (
	flagMu      sync.RWMutex
	allowedFlag *flag.Flag
)

// defaultRepos is the allowlist: --allowed-repos and NSELF_CI_ALLOWED_REPOS, plus every repository that has
// a per-repo webhook secret (the operator named it there). Names are deduplicated case-insensitively.
func defaultRepos(ctx context.Context) ([]string, error) {
	var names []string
	flagMu.RLock()
	if allowedFlag != nil {
		names = append(names, splitList(allowedFlag.Value.String())...)
	}
	flagMu.RUnlock()
	names = append(names, splitList(os.Getenv("NSELF_CI_ALLOWED_REPOS"))...)
	if eff, err := currentPolicy(ctx); err != nil {
		return nil, err
	} else if eff != nil {
		secrets, err := webhook.SecretsFrom(eff)
		if err != nil {
			return nil, err
		}
		for repo := range secrets {
			names = append(names, repo)
		}
	}
	return cleanRepos(names), nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// cleanRepos keeps valid owner/repo names, one per lower-case form, sorted.
func cleanRepos(names []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range names {
		k := strings.ToLower(n)
		if seen[k] || !coordinator.ValidProject(n) || strings.Count(n, "/") != 1 {
			continue
		}
		seen[k] = true
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func init() {
	coordinator.RegisterFlag(func(fs *flag.FlagSet) {
		flagMu.Lock()
		allowedFlag = fs.Lookup("allowed-repos")
		flagMu.Unlock()
	})
}

// marshal is json.Marshal for values that cannot fail.
func marshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
