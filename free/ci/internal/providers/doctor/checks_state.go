package doctor

// Purpose: CI-DP008 to CI-DP014, the checks that read stored facts and policy values: webhook secret
// variables, the fork-approval fact, provider health, the Forgejo URL, registration errors and the
// hosted authorization of the project.
// Inputs:  Config, the environment reader and the CI store (nil when none exists).
// Outputs: doctor.Check values; DP008 and DP009 give one failing check per project, DP011 one per
// provider API.
// Constraints: ids: DP008 GitHub webhook secret variables, DP009 GitLab webhook secret variables,
// DP010 fork-approval policy against just-in-time runners, DP011 provider health, DP012 Forgejo URL,
// DP013 registration errors, DP014 hosted authorization. A variable's value is never read beyond
// "is it non-empty"; health is read from the stored breaker fact (no request is sent).

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/forgejo"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/hosted"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/jit"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/pipeline"
	"github.com/nself-org/plugins/free/ci/internal/providers/health"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
)

// factProvider is the provider name the JIT registrar files the fork-approval fact under.
const factProvider = "github"

// factMaxAge is how long a recorded fork-approval policy is trusted before the doctor asks for a new read.
const factMaxAge = 30 * 24 * time.Hour

type secretRef struct{ subject, env string }

func sortedRefs(m map[string]string) []secretRef {
	out := make([]secretRef, 0, len(m))
	for subject, env := range m {
		out = append(out, secretRef{subject, env})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].subject < out[j].subject })
	return out
}

// secretChecks makes the pass-or-one-failure-per-project result of a webhook secret table.
func (s *state) secretChecks(id, name, what string, refs []secretRef) []doctor.Check {
	var out []doctor.Check
	for _, r := range refs {
		if strings.TrimSpace(s.env.Getenv(r.env)) == "" {
			c := mk(id, name, doctor.Fail, what+" for "+r.subject+" is read from "+r.env+", which is empty or unset: its deliveries are refused", "Export "+r.env+" with the secret configured on the webhook, or remove the row with `nself ci policy set`")
			c.Subject = r.subject
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		msg := "no per-project " + what + " configured"
		if len(refs) > 0 {
			msg = "all " + what + " variables are set"
		}
		out = append(out, mk(id, name, doctor.Pass, msg, ""))
	}
	return out
}

func (s *state) checkWebhookSecrets() []doctor.Check {
	gh := map[string]string{}
	for repo, rs := range s.cfg.GitHubSecrets {
		gh[repo] = rs.Env
	}
	out := s.secretChecks("CI-DP008", "github webhook secrets", "webhook secret", sortedRefs(gh))
	gl := map[string]string{}
	for project, rs := range s.cfg.GitLabSecrets.Signing {
		gl[project+" (signing)"] = rs.Env
	}
	for project, rs := range s.cfg.GitLabSecrets.Tokens {
		gl[project+" (token)"] = rs.Env
	}
	return append(out, s.secretChecks("CI-DP009", "gitlab webhook secrets", "webhook secret", sortedRefs(gl))...)
}

func (s *state) checkForkApproval(ctx context.Context) doctor.Check {
	const id, name = "CI-DP010", "fork pull request approval"
	repo, ok := GitHubRepo(ctx, s.env.Root, s.env.Capture)
	switch {
	case !s.cfg.JIT:
		return mk(id, name, doctor.Pass, "just-in-time runners are not enabled", "")
	case !ok:
		return mk(id, name, doctor.Pass, "no GitHub repository to check", "")
	case s.st == nil:
		return mk(id, name, doctor.Warn, "just-in-time runners are enabled and there is no state database to hold the fork-approval policy of "+repo,
			"Run `nself ci providers test github-hosted`, which records it")
	}
	f, found, err := s.st.ProviderFactGet(ctx, factProvider, repo, jit.FactApprovalPolicy)
	if err != nil || !found {
		return mk(id, name, doctor.Warn, "just-in-time runners are enabled and the fork pull request approval policy of "+repo+" is not recorded, so it is unknown",
			"Run `nself ci providers test github-hosted --repo "+repo+"`: it reads the policy from GitHub and records it")
	}
	var v struct {
		Policy string `json:"approval_policy"`
	}
	if json.Unmarshal(f.Value, &v) != nil || v.Policy == "" {
		return mk(id, name, doctor.Warn, "the stored fork-approval fact for "+repo+" is unreadable", "Run a just-in-time dispatch to record it again")
	}
	age := time.Since(f.ObservedAt)
	switch {
	case age > factMaxAge:
		return mk(id, name, doctor.Warn, "the fork pull request approval policy of "+repo+" was recorded "+f.ObservedAt.UTC().Format("2006-01-02")+" and may have changed since",
			"Run `nself ci providers test github-hosted --repo "+repo+"` to read it again")
	case v.Policy == jit.StrictApproval:
		return mk(id, name, doctor.Pass, repo+" requires approval for all external contributors (recorded "+f.ObservedAt.UTC().Format("2006-01-02")+")", "")
	}
	return mk(id, name, doctor.Warn, repo+" approves fork pull requests under policy "+v.Policy+", weaker than "+jit.StrictApproval+", while just-in-time runners are enabled: a public repository's fork can run code on your runner",
		"In the repository settings (Actions, General, Fork pull request workflows) require approval for all external contributors, or disable just-in-time runners for it")
}

func (s *state) checkHealth(ctx context.Context) []doctor.Check {
	opts := health.Options{}
	if s.st != nil {
		opts.Store = s.st
	}
	tracker := health.New(opts)
	seen := map[string]bool{}
	var out []doctor.Check
	for _, p := range providers.All() {
		fam := health.Family(p.ID())
		if seen[fam] {
			continue
		}
		seen[fam] = true
		h := tracker.HealthOf(ctx, p.ID())
		c := mk("CI-DP011", "provider health", doctor.Pass, fam+" API: "+h.State, "")
		c.Subject = fam
		switch h.State {
		case "down":
			c.Status, c.Message = doctor.Fail, fam+" API is down ("+h.Reason+"): hosted runners of "+fam+" are offline"
			c.Remediation = "Check the provider's status page; the breaker probes again by itself and `nself ci providers test " + p.ID() + "` shows the live answer"
		case "degraded":
			c.Status, c.Message = doctor.Warn, fam+" API is degraded ("+h.Reason+")"
			c.Remediation = "Wait for the retry time or check the provider's rate limits with `nself ci providers show " + p.ID() + "`"
		}
		out = append(out, c)
	}
	return out
}

func (s *state) checkForgejo() doctor.Check {
	const id, name = "CI-DP012", "forgejo url"
	if !s.cfg.ForgejoSet {
		return mk(id, name, doctor.Pass, "providers.forgejo.url is not set", "")
	}
	c := mk(id, name, doctor.Pass, "providers.forgejo.url is "+s.cfg.ForgejoURL+" (the live probe is `nself ci forgejo`, removed in v1.6)", "")
	if u, err := url.Parse(s.cfg.ForgejoURL); err == nil && u.Scheme == "http" && !localHost(u.Hostname()) {
		c.Status, c.Message = doctor.Warn, "providers.forgejo.url sends the admin password over plain http to "+u.Hostname()
		c.Remediation = "Use an https URL for a Forgejo server that is not on this machine"
	}
	return c
}

func localHost(h string) bool { return h == "localhost" || h == "127.0.0.1" || h == "::1" }

func (s *state) checkRegistration() doctor.Check {
	var errs []error
	errs = append(errs, runtime.RegistrationErrors()...)
	errs = append(errs, hosted.RegistrationErrors()...)
	errs = append(errs, jit.RegistrationErrors()...)
	errs = append(errs, pipeline.RegistrationErrors()...)
	errs = append(errs, forgejo.RegistrationErrors()...)
	if len(errs) == 0 {
		return mk("CI-DP013", "provider registration", doctor.Pass, "every provider and policy key registered", "")
	}
	msgs := make([]string, 0, len(errs))
	for _, e := range errs {
		msgs = append(msgs, oneLine(e.Error()))
	}
	return mk("CI-DP013", "provider registration", doctor.Fail, "a provider or policy key did not register: "+strings.Join(msgs, "; "), "This is a build problem; report it with this line")
}

func (s *state) checkAuthorization() doctor.Check {
	const id, name = "CI-DP014", "hosted authorization"
	zone := s.cfg.PrivacyZone
	if zone == "" {
		return mk(id, name, doctor.Pass, "the privacy zone could not be resolved", "")
	}
	list := "[" + strings.Join(s.cfg.Authorized, ", ") + "]"
	c := mk(id, name, doctor.Pass, "trust.privacy_zone is "+zone+"; trust.providers is "+list, "")
	if zone == "provider-allowlist" && len(s.cfg.Authorized) == 0 {
		c.Status, c.Message = doctor.Warn, "trust.privacy_zone is provider-allowlist and trust.providers is empty: no hosted provider is admissible"
		c.Remediation = "Authorize one with `nself ci providers authorize <id> --project <project>`"
	}
	return c
}
