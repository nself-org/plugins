package doctor

// Purpose: CI-DP001 to CI-DP005, the credential and policy checks of the providers section.
// Inputs:  Config, the environment reader and the exec funnel (through the state in section.go).
// Outputs: doctor.Check values; none carries a credential value.
// Constraints: ids: DP001 GitHub credential source, DP002 GitHub credential kind (the review F18
// warning), DP003 GitLab token, DP004 GitLab runner token, DP005 the providers.* keys resolve.

import (
	"context"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
)

func mk(id, name string, status doctor.Status, msg, remediation string) doctor.Check {
	return doctor.Check{ID: id, Name: name, Status: status, Message: msg, Remediation: remediation}
}

func (s *state) checkPolicy() doctor.Check {
	const id, name = "CI-DP005", "providers policy"
	if s.cfg.Err == nil {
		return mk(id, name, doctor.Pass, "the providers.* keys resolve", "")
	}
	status := doctor.Warn
	if strings.Contains(s.cfg.Err.Error(), "E67") { // a key the operator set does not decode, or a scope loosens another
		status = doctor.Fail
	}
	return mk(id, name, status, "operator policy: "+oneLine(s.cfg.Err.Error())+"; the defaults are used for the checks below",
		"Run `nself ci policy show` to see the rows, then fix the key with `nself ci policy set` (a repository cannot set these keys)")
}

func (s *state) checkGitHubCredential(ctx context.Context) (doctor.Check, doctor.Check) {
	cred := GitHubCredential(ctx, s.cfg, s.env.Getenv, s.env.Capture)
	src := mk("CI-DP001", "github credential", doctor.Pass, "GitHub credential: "+cred.Source, "")
	kind := mk("CI-DP002", "github credential kind", doctor.Pass, "no GitHub credential to classify", "")
	switch {
	case !cred.Present && cred.Env != "":
		src.Status, src.Message = doctor.Fail, "providers.github.token_env names "+cred.Env+", which is empty or unset"
		src.Remediation = "Export " + cred.Env + " with a fine-grained token (github_pat_…) or run `nself ci policy set --scope user providers.github.token_env \"\"` to use gh's login"
	case !cred.Present:
		src.Status, src.Message = doctor.Warn, "gh is not authenticated for "+GitHubHost(s.cfg.Runtime.GitHubAPIURL)+" and providers.github.token_env is not set: GitHub-hosted capacity is unavailable"
		src.Remediation = "Run `gh auth login`, or export a fine-grained token and name its variable with `nself ci policy set --scope user providers.github.token_env NAME`"
	default:
		kind.Message = "GitHub credential kind: " + cred.Kind + " (" + cred.Source + ")"
		if WeakGitHubKind(cred.Kind) {
			kind.Status = doctor.Warn
			kind.Message = "the GitHub credential is " + map[string]string{KindClassic: "a classic personal access token (ghp_)", KindOAuth: "gh's OAuth login (gho_)"}[cred.Kind] + " with broader access than a fine-grained token (github_pat_)"
			kind.Remediation = "Create a fine-grained token limited to the repositories CI uses (Actions, Contents and Metadata read, Administration read), export it and name the variable with `nself ci policy set --scope user providers.github.token_env NAME`"
		}
	}
	return src, kind
}

func (s *state) checkGitLabCredentials() (doctor.Check, doctor.Check) {
	tok := mk("CI-DP003", "gitlab token", doctor.Pass, "GitLab is not configured", "")
	run := mk("CI-DP004", "gitlab runner token", doctor.Pass, "providers.gitlab.runner_token_env is not set (runner registration is off)", "")
	if !GitLabConfigured(s.cfg) {
		return tok, run
	}
	if c := GitLabCredential(s.cfg, s.env.Getenv, false); c.Present {
		tok.Message = "GitLab token: " + c.Source
	} else {
		tok.Status, tok.Message = doctor.Fail, "providers.gitlab.token_env names "+c.Env+", which is empty or unset"
		tok.Remediation = "Export " + c.Env + " with a token that has api scope and the Developer role on the project"
	}
	if c := GitLabCredential(s.cfg, s.env.Getenv, true); c.Source != "none" {
		if c.Present {
			run.Message = "GitLab runner token: " + c.Source
		} else {
			run.Status, run.Message = doctor.Fail, "providers.gitlab.runner_token_env names "+c.Env+", which is empty or unset"
			run.Remediation = "Export " + c.Env + " with a token that has create_runner scope, or unset the key"
		}
	}
	return tok, run
}

// oneLine collapses white space so a multi-line error stays on one report line.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
