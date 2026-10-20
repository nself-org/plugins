package pipeline

// Purpose: Dispatch (Epic D11): validate one attempt, choose the ref, journal the intent, create the
//          pipeline through POST /projects/:id/pipeline and return the handle of the one pipeline it
//          started.
// Inputs:  providers.Attempt (project path "group/.../name", admitted revision, fetch ref, spec, trust,
//          runner tag, compat mode, timeout).
// Outputs: providers.Handle{Provider, RunID "group/.../name/<id>", Nonce, Ref}.
// Constraints: the revision sent is the admitted one and no ref is resolved to a revision; the only
//          lookups before the request are the default branch (trusted work only), the instance version
//          and the main CI file of the ref (inputs.go: for untrusted work it must be a generated root); a 400 from the create call is told
//          apart by one branch lookup (missing ref) before it is blamed on the pipeline variables; untrusted work (and any trust class this build does not know) dispatches on the
//          untrusted ref; the values travel as pipeline `inputs` when the instance takes them (GitLab
//          17.10 and later) and the main CI file declares them, else as variables; anything outside the allowlists is
//          E708 before a request is made; the encoded spec is at most MaxSpecChars (the documented
//          limit of a pipeline variable value); the intent is journalled before the request; the
//          create call is sent once: a transient failure looks the pipeline up by nonce and adopts it
//          instead of resending; the trigger endpoint is never used and no token is in a URL.

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// dispatchRef is the ref the pipeline runs from: the default branch for trusted work, the untrusted
// ref for everything else.
func (p *Provider) dispatchRef(ctx context.Context, c *rest.Client, r request) (string, error) {
	if !r.trusted {
		if !validRef(p.cfg.UntrustedRef) {
			return "", api.Misconfig("untrusted_ref")
		}
		return p.cfg.UntrustedRef, nil
	}
	var info api.ProjectInfo
	if _, err := c.Do(ctx, "GET", api.ProjectPath, r.project.Params(), nil, &info); err != nil {
		return "", err
	}
	if !validRef(info.DefaultBranch) {
		return "", api.Misconfig("default_branch")
	}
	return info.DefaultBranch, nil
}

func validRef(s string) bool {
	return refRE.MatchString(s) && !strings.HasPrefix(s, "refs/") && !strings.Contains(s, "..") && !strings.Contains(s, "//") &&
		!strings.HasSuffix(s, "/") && !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, ".lock")
}

// mode asks the instance for its version and the main CI file for its inputs: inputs when the instance
// is 17.10 or later and the main file at ref declares them (inputs.go), variables otherwise (and when
// the version cannot be read for a reason that is not worth retrying).
func (p *Provider) mode(ctx context.Context, c *rest.Client, project api.Project, ref string, trusted bool) (string, error) {
	var v struct {
		Version string `json:"version"`
	}
	if _, err := c.Do(ctx, "GET", api.VersionPath, nil, nil, &v); err != nil {
		if api.IsTransient(err) || api.IsCredentialMissing(err) {
			return "", err
		}
		v.Version = "" // not readable: no inputs
	}
	if !trusted {
		return p.untrustedMode(ctx, c, project, ref, v.Version)
	}
	if !api.SupportsInputs(v.Version) {
		return ModeVariables, nil
	}
	declared, err := p.mainDeclares(ctx, c, project, ref)
	if err != nil {
		return "", err
	}
	if declared {
		return ModeInputs, nil
	}
	return ModeVariables, nil
}

// Dispatch implements providers.Dispatcher.
func (p *Provider) Dispatch(ctx context.Context, a providers.Attempt) (providers.Handle, error) {
	if !compat.V15() { // compat.V15(P7-HOST-06): no pipeline dispatch -> GitLab pipeline API
		return providers.Handle{}, api.Refuse("hosted dispatch needs v1.5")
	}
	r, err := plan(a)
	if err != nil {
		return providers.Handle{}, err
	}
	c, err := p.client(ctx)
	if err != nil {
		return providers.Handle{}, err
	}
	ref, err := p.dispatchRef(ctx, c, r)
	if err != nil {
		return providers.Handle{}, err
	}
	mode, err := p.mode(ctx, c, r.project, ref, r.trusted)
	if err != nil {
		return providers.Handle{}, err
	}
	fresh, err := p.cfg.Nonce()
	if err != nil {
		return providers.Handle{}, err
	}
	start := p.cfg.Clock()
	stand, previous, err := p.jrnl().Begin(ctx, Record{Attempt: a.ID, Lineage: Lineage(a.ID), N: a.N, Revision: r.revision, SpecDigest: r.digest, Nonce: fresh, Ref: ref, Mode: mode, Project: r.project.Path, Trusted: r.trusted})
	if err != nil {
		if errors.Is(err, errSuperseded) {
			return providers.Handle{}, api.Refuse("attempt.superseded")
		}
		if errors.Is(err, errConflict) {
			return providers.Handle{}, api.Refuse("attempt.conflict")
		}
		return providers.Handle{}, api.Unavailable("journal")
	}
	if previous != nil { // a retry never leaves the previous attempt's pipeline running
		p.cancelNonce(ctx, c, r.project, *previous, start)
	}
	h := providers.Handle{Provider: p.ID(), Nonce: stand.Nonce, Ref: stand.Ref}
	if stand.Nonce != fresh { // this attempt number was begun before: its pipeline may exist already
		if id, found, ferr := p.awaitPipeline(ctx, c, r.project, stand, start.Add(-leaseTTL-window), start.Add(window)); ferr != nil {
			return providers.Handle{}, ferr
		} else if found {
			h.RunID = runHandle(r.project, id)
			return h, nil
		}
	}
	return p.send(ctx, c, r, h, stand.Mode, start)
}

var retrySuffix = regexp.MustCompile(`(-retry-[0-9]+|\.a[0-9]+)$`)

// Lineage is the attempt id without the suffixes a retry adds ("<id>-retry-2" from the lease store,
// "<row>.a2" from the bounded infra retry), so every retry of one job shares one journal line and the
// retry cancels the pipeline of the attempt it replaces.
func Lineage(attemptID string) string {
	for {
		trimmed := retrySuffix.ReplaceAllString(attemptID, "")
		if trimmed == attemptID || trimmed == "" {
			return attemptID
		}
		attemptID = trimmed
	}
}

// send creates the pipeline once and resolves its id from the answer or by nonce.
func (p *Provider) send(ctx context.Context, c *rest.Client, r request, h providers.Handle, mode string, start time.Time) (providers.Handle, error) {
	var out api.Pipeline
	_, err := c.Do(ctx, "POST", api.CreatePipeline, r.project.Params(), r.body(h.Ref, h.Nonce, mode), &out)
	switch {
	case err == nil && out.ID > 0:
		h.RunID = runHandle(r.project, out.ID)
		return h, nil
	case err != nil && !api.IsTransient(err):
		if api.IsStatus(err, 400) {
			switch {
			case p.refMissing(ctx, c, r.project, h.Ref):
				return providers.Handle{}, api.Misconfig("ref.missing: create the branch " + h.Ref + " (nself ci providers gitlab init --check prints the command)")
			case mode == ModeVariables:
				return providers.Handle{}, api.Misconfig(RemediationVariables)
			}
		}
		return providers.Handle{}, err
	}
	rec := Record{Nonce: h.Nonce, Ref: h.Ref, Mode: mode}
	id, found, ferr := p.awaitPipeline(ctx, c, r.project, rec, start.Add(-window), start.Add(window))
	switch {
	case found:
		h.RunID = runHandle(r.project, id)
		return h, nil
	case ferr != nil:
		return providers.Handle{}, ferr
	case err != nil:
		return providers.Handle{}, err
	}
	return providers.Handle{}, api.Unavailable("run.not_found")
}
