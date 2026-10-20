// Package status is the GitLab commit-status adapter (P7-HOST-06, Epic D7/D11): the one report.Reporter
// for GitLab remotes. It posts name "nself-ci" to POST /projects/:id/statuses/:sha through the provider
// REST funnel and registers itself with report.RegisterReporter, so the outbox sender routes a GitLab
// remote here and a GitHub remote never.
package status

// Purpose: deliver one report.Status to GitLab.
// Inputs:  report.Status (owner, repo, revision, state, description, target url); the operator's
//          provider policy (api_url) for the host and the "gitlab" credential rule for the token.
// Outputs: a posted commit status, or a providers.Error from the funnel.
// Constraints: there is one Reporter interface (report.Reporter) and one status name (report.Context,
//          "nself-ci"); the four report states map to pending, success, failed and failed, and a caller
//          that knows more (the evidence projection says running, or the run was cancelled) supplies a
//          Phase hook, which refines pending to running and failed to canceled; the description is the
//          one CI-37's Publisher built, cut to 140 characters; a target URL is passed only when the
//          Status has one; the project is "owner/repo" of the Status (a nested group is not reachable
//          through report.Status, whose Validate refuses a slash); the token never appears in a URL,
//          an error or a log; the matcher is pure: it reads the remote text and the policy's api_url
//          host and calls nothing else, and it matches only when compat.V15() (v1.4 has no GitLab
//          reporter).

import (
	"context"
	"net/url"
	"strings"
	"sync"

	"github.com/nself-org/cli/sdk/go/v2/compat"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// Kind is the reporter kind registered with the report registry.
const Kind = "gitlab"

// The GitLab states.
const (
	Pending  = "pending"
	Running  = "running"
	Success  = "success"
	Failed   = "failed"
	Canceled = "canceled"
)

// Phase refines a report state with what the evidence projection knows.
type Phase string

// The phases a Phase hook may return; "" refines nothing.
const (
	PhaseNone     Phase = ""
	PhaseRunning  Phase = "running"
	PhaseCanceled Phase = "canceled"
)

// Map turns a report state and a phase into the GitLab state. An unknown state is failed, never
// success, so a bad value cannot turn a gate green.
func Map(s report.State, p Phase) string {
	switch s {
	case report.Success:
		return Success
	case report.Pending:
		if p == PhaseRunning {
			return Running
		}
		return Pending
	case report.Failure, report.Error:
		if p == PhaseCanceled {
			return Canceled
		}
		return Failed
	}
	return Failed
}

// Reporter posts statuses to GitLab.
type Reporter struct {
	// Client builds the REST client of one post; nil builds it from the operator's policy.
	Client func(context.Context) (*rest.Client, error)
	// Phase refines the state of a status; nil refines nothing.
	Phase func(report.Status) Phase
}

var _ report.Reporter = (*Reporter)(nil)

type body struct {
	State       string `json:"state"`
	Name        string `json:"name"`
	Description string `json:"description"`
	TargetURL   string `json:"target_url,omitempty"`
}

// body builds the request body of a status.
func (r *Reporter) body(s report.Status) body {
	var ph Phase
	phaseMu.RLock()
	hook := r.Phase
	phaseMu.RUnlock()
	if hook != nil {
		ph = hook(s)
	}
	return body{State: Map(s.State, ph), Name: report.Context, Description: report.Truncate(s.Description), TargetURL: s.TargetURL}
}

// Post implements report.Reporter.
func (r *Reporter) Post(ctx context.Context, s report.Status) error {
	if err := s.Validate(); err != nil {
		return err
	}
	project, ok := api.ParseProject(s.Owner + "/" + s.Repo)
	if !ok {
		return api.Misconfig("project")
	}
	c, err := r.client(ctx)
	if err != nil {
		return err
	}
	_, err = c.Do(ctx, "POST", api.StatusPath, project.With("sha", s.Revision), r.body(s), nil)
	return err
}

func (r *Reporter) client(ctx context.Context) (*rest.Client, error) {
	if r.Client != nil {
		return r.Client(ctx)
	}
	return api.NewClient(ctx, "nself-ci-gitlab-status")
}

// remoteHost extracts the host of a git remote: https://host/path, ssh://git@host/path or git@host:path.
func remoteHost(remote string) string {
	remote = strings.TrimSpace(remote)
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil {
			return ""
		}
		return strings.ToLower(u.Hostname())
	}
	if at := strings.IndexByte(remote, '@'); at >= 0 {
		rest := remote[at+1:]
		if colon := strings.IndexByte(rest, ':'); colon > 0 {
			return strings.ToLower(rest[:colon])
		}
	}
	return ""
}

// Match reports whether remote lives on the GitLab host of providers.gitlab.api_url. A policy that
// cannot be read matches nothing, so a broken policy never captures a GitHub remote.
func Match(remote string) bool {
	if !compat.V15() { // compat.V15(P7-HOST-06): no GitLab reporter -> v1.4 behaviour, a GitLab remote has none
		return false
	}
	s, err := runtime.CurrentSettings(context.Background())
	if err != nil {
		return false
	}
	u, err := url.Parse(s.GitLabAPIURL)
	if err != nil || u.Hostname() == "" {
		return false
	}
	h := remoteHost(remote)
	return h != "" && h == strings.ToLower(u.Hostname())
}

// RegistrationError is what init() could not register, if anything.
var RegistrationError error

var registered = &Reporter{}

func init() { RegistrationError = report.RegisterReporter(Kind, Match, registered) }

var phaseMu sync.RWMutex

// UsePhase installs the hook that refines pending to running and failed to canceled for the registered
// reporter (the coordinator start calls it with a function that reads the run's projection; with none
// installed only pending, success and failed are posted). Passing nil removes it.
func UsePhase(fn func(report.Status) Phase) {
	phaseMu.Lock()
	defer phaseMu.Unlock()
	registered.Phase = fn
}

// PhaseOf is the phase the installed hook gives a status (PhaseNone with no hook).
func PhaseOf(s report.Status) Phase {
	phaseMu.RLock()
	hook := registered.Phase
	phaseMu.RUnlock()
	if hook == nil {
		return PhaseNone
	}
	return hook(s)
}
