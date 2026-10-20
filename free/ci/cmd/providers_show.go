package main

// Purpose: `nself ci providers show <id> [--repo R] [--json]` (P7-HOST-12, Epic D14): one provider's
// capabilities (Describe), health, credential, authorization, economics with their confidence (the
// same figures as `ci policy usage`, SCHED-07) and the facts the host-provider store holds for it.
// Inputs:  the provider id, the repository (flag, else the origin remote), the operator policy.
// Outputs: a text block or one v1 JSON envelope; exit 0, or 1 for an unknown provider.
// Constraints: read only; Describe may send one repository lookup (the provider's own rule), and its
//          failure is reported in the output, not as a failed command; stored fact values are opaque
//          JSON the providers wrote and never hold a credential; text goes through plainText.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	cidoc "github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	pdoctor "github.com/nself-org/plugins/free/ci/internal/providers/doctor"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/hosted"
	"github.com/nself-org/plugins/free/ci/internal/providers/health"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type capJSON struct {
	ID       string `json:"id"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	State    string `json:"state"`
	Class    string `json:"economics_class"`
	Privacy  string `json:"privacy_zone"`
	Isolated string `json:"isolation"`
}

type factJSON struct {
	Scope      string          `json:"scope"`
	Key        string          `json:"key"`
	ObservedAt string          `json:"observed_at"`
	Value      json.RawMessage `json:"value"`
}

type authJSON struct {
	PrivacyZone string   `json:"privacy_zone"`
	Providers   []string `json:"providers"`
	Authorized  bool     `json:"authorized"`
}

type showJSON2 struct {
	ID            string                  `json:"id"`
	Family        string                  `json:"family"`
	Repo          string                  `json:"repo,omitempty"`
	Health        healthJSON              `json:"health"`
	Credential    *pdoctor.Credential     `json:"credential,omitempty"`
	Authorization authJSON                `json:"authorization"`
	Capabilities  []capJSON               `json:"capabilities"`
	DescribeError string                  `json:"describe_error,omitempty"`
	Economics     *economics.ProviderLine `json:"economics,omitempty"`
	EconomicsNote string                  `json:"economics_note,omitempty"`
	Facts         []factJSON              `json:"facts"`
}

func factStr(f model.Fact[string]) string {
	if f.Value == nil {
		return "unknown"
	}
	return *f.Value
}

func capSummary(c model.Capability) capJSON {
	out := capJSON{ID: c.Identity.ID, OS: factStr(c.Platform.OS), Arch: factStr(c.Platform.Arch), State: factStr(c.Availability.State), Class: factStr(c.Economics.Class), Privacy: "unknown", Isolated: "unknown"}
	if v := c.Trust.PrivacyZone.Value; v != nil {
		out.Privacy = string(*v)
	}
	if v := c.Trust.Isolation.Value; v != nil {
		out.Isolated = string(*v)
	}
	return out
}

// useOperatorPolicy installs cfg's effective policy as the policy source of the provider packages for
// the life of the command (the coordinator does this once at start); the returned func removes it.
func useOperatorPolicy(cfg pdoctor.Config) func() {
	src := func(context.Context) (*policy.Effective, error) {
		if cfg.Effective == nil {
			return nil, cfg.Err
		}
		return cfg.Effective, nil
	}
	runtime.UsePolicy(src)
	hosted.UsePolicy(src)
	return func() { runtime.UsePolicy(nil); hosted.UsePolicy(nil) }
}

func showProvider(ctx context.Context, env providersEnv, cfg pdoctor.Config, st *store.Store, root, id string, f providersFlags) int {
	const command = "ci providers show"
	p, ok := providers.Lookup(id)
	if !ok {
		return adminFail(env.adminEnv, command, f.json, unknownProvider(id))
	}
	defer useOperatorPolicy(cfg)()
	repo := f.repo
	if repo == "" && health.Family(id) == "github" {
		repo, _ = pdoctor.GitHubRepo(ctx, root, cidoc.CaptureFunc(env.Run))
	}
	d := showJSON2{ID: id, Family: health.Family(id), Repo: repo, Health: healthOf(ctx, st, id), Credential: credentialOf(ctx, env, cfg, id),
		Authorization: authJSON{PrivacyZone: cfg.PrivacyZone, Providers: append([]string{}, cfg.Authorized...), Authorized: slices.Contains(cfg.Authorized, id)},
		Capabilities:  []capJSON{}, Facts: []factJSON{}}
	caps, err := p.Describe(ctx, providers.Query{Project: policy.ProjectIDGit(ctx, env.Run, root), Repo: repo, Visibility: "unknown"})
	if err != nil {
		d.DescribeError = oneLineMsg(err)
	}
	for _, c := range caps {
		d.Capabilities = append(d.Capabilities, capSummary(c))
	}
	if rep, rerr := usageReport(ctx, root, st, env.Now()); rerr != nil {
		d.EconomicsNote = "the economics report is unavailable: " + oneLineMsg(rerr)
	} else {
		for i := range rep.Providers {
			if rep.Providers[i].Provider == id {
				d.Economics = &rep.Providers[i]
			}
		}
		if d.Economics == nil {
			d.EconomicsNote = "no spend recorded for " + id
		}
	}
	d.Facts = storedFacts(ctx, st, d.Family, repo)
	if f.json {
		return adminWriteData(env.adminEnv, command, d)
	}
	printShow(env, d)
	return 0
}

func storedFacts(ctx context.Context, st *store.Store, family, repo string) []factJSON {
	out := []factJSON{}
	if st == nil {
		return out
	}
	for _, scope := range []string{"health", repo} {
		if scope == "" {
			continue
		}
		facts, err := st.ProviderFactList(ctx, family, scope, "")
		if err != nil {
			continue
		}
		for _, f := range facts {
			out = append(out, factJSON{Scope: f.Scope, Key: f.Key, ObservedAt: f.ObservedAt.UTC().Format("2006-01-02T15:04:05Z"), Value: f.Value})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Scope+"/"+out[i].Key < out[j].Scope+"/"+out[j].Key })
	return out
}

func printShow(env providersEnv, d showJSON2) {
	w := func(format string, a ...any) { _, _ = fmt.Fprintf(env.Out, format, a...) }
	w("provider      %s (family %s)\n", plainText(d.ID), d.Family)
	w("health        %s %s\n", d.Health.State, plainText(d.Health.Reason))
	if c := d.Credential; c != nil {
		w("credential    %s, %s, kind %s\n", plainText(c.Source), map[bool]string{true: "set", false: "missing"}[c.Present], plainText(c.Kind))
	}
	w("authorized    %v (trust.privacy_zone %s, trust.providers %v)\n", d.Authorization.Authorized, d.Authorization.PrivacyZone, d.Authorization.Providers)
	if d.Repo != "" {
		w("repository    %s\n", plainText(d.Repo))
	}
	for _, c := range d.Capabilities {
		w("capability    %s  %s/%s  %s  economics %s  isolation %s  privacy %s\n", plainText(c.ID), c.OS, c.Arch, c.State, c.Class, c.Isolated, c.Privacy)
	}
	if d.DescribeError != "" {
		w("describe      %s\n", plainText(d.DescribeError))
	} else if len(d.Capabilities) == 0 {
		w("describe      no capability (pass --repo to describe one repository)\n")
	}
	if e := d.Economics; e != nil {
		w("economics     class %s, local %s\n", e.Class, fig(e.Local))
		for _, row := range []struct {
			name string
			f    *economics.Figure
		}{{"rate/hour", e.RatePerHour}, {"rate/minute", e.RatePerMinute}, {"ceiling", e.Ceiling}, {"imported", e.Imported}, {"allowance", e.Allowance}} {
			if row.f != nil {
				w("  %-12s %s\n", row.name, fig(*row.f))
			}
		}
	} else if d.EconomicsNote != "" {
		w("economics     %s\n", plainText(d.EconomicsNote))
	}
	for _, f := range d.Facts {
		w("fact          %s/%s %s %s\n", plainText(f.Scope), plainText(f.Key), f.ObservedAt, plainText(string(f.Value)))
	}
}

func unknownProvider(id string) error {
	var ids []string
	for _, p := range providers.All() {
		ids = append(ids, p.ID())
	}
	return adminUsage("unknown provider %q (registered: %v)", id, ids)
}

func oneLineMsg(err error) string { return plainText(fmt.Sprint(err)) }
