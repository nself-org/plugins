package hosted

// Purpose: Dispatch (Epic D5): validate one attempt, choose the ref, journal the intent, send the
//          workflow_dispatch request and return the handle of the one run it started.
// Inputs:  providers.Attempt (repo "owner/name", admitted revision, fetch ref, spec, trust, label,
//          compat mode, timeout).
// Outputs: providers.Handle{Provider, RunID "owner/name/<id>", Nonce, Ref}.
// Constraints: the revision sent is the admitted one and no ref is ever resolved to a revision;
//          the only lookup before the request is the default branch name, and only for trusted work;
//          untrusted work (and any trust class this build does not know) dispatches on
//          providers.github.untrusted_ref; runner_label, compat_mode and timeout_minutes are mapped
//          onto the workflow's choice options and anything else is E708 before a request is made;
//          the spec is gzip then base64 and at most 48 KiB encoded; the intent is journalled before
//          the request (see journal.go); the body asks for return_run_details; the request is sent
//          once: a transient failure looks the run up by nonce and adopts it instead of resending.

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	githubtmpl "github.com/nself-org/plugins/free/ci/templates/github"
)

// MaxSpecBytes is the largest encoded spec the workflow input carries.
const MaxSpecBytes = 48 << 10

const (
	defaultLabel   = "ubuntu-24.04"
	defaultTimeout = 60
)

var (
	revisionRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
	fetchRefRE = regexp.MustCompile(`^refs/pull/[0-9]+/head$`)
)

// request is one validated dispatch.
type request struct {
	repo                                                repoRef
	attempt, revision, fetchRef, label, compat, timeout string
	spec, digest                                        string
	n                                                   int
	trusted                                             bool
}

func trustedClass(t string) bool { return t == "owner" || t == "internal" || t == "collaborator" }

func timeoutOption(ms int64) (string, bool) {
	if ms <= 0 {
		return "60", true
	}
	minutes := (ms + 59999) / 60000
	for _, o := range githubtmpl.Timeouts {
		if int64(o) >= minutes {
			return strconv.Itoa(o), true
		}
	}
	return "", false
}

func encodeSpec(spec []byte) (string, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return "", refuse("spec.encode")
	}
	if _, err = zw.Write(spec); err != nil || zw.Close() != nil {
		return "", refuse("spec.encode")
	}
	enc := base64.StdEncoding.EncodeToString(buf.Bytes())
	if len(enc) > MaxSpecBytes {
		return "", refuse("spec.too_large")
	}
	return enc, nil
}

func plan(a providers.Attempt) (request, error) {
	r := request{attempt: a.ID, revision: a.Revision, fetchRef: a.FetchRef, label: a.Label, compat: a.CompatMode, n: a.N}
	repo, ok := parseRepo(a.Repo)
	switch {
	case !ok:
		return r, refuse("repo")
	case a.ID == "" || len(a.ID) > 256 || strings.ContainsAny(a.ID, "\r\n\x00"):
		return r, refuse("attempt")
	case !revisionRE.MatchString(a.Revision):
		return r, refuse("revision")
	case a.FetchRef != "" && !fetchRefRE.MatchString(a.FetchRef):
		return r, refuse("fetch_ref")
	}
	r.repo = repo
	if r.label == "" {
		r.label = defaultLabel
	}
	names, err := githubtmpl.LabelNames()
	if err != nil {
		return r, misconfig("labels")
	}
	if !contains(names, r.label) {
		return r, refuse("label.not_allowed")
	}
	if r.compat == "" {
		r.compat = "v1.5"
	}
	if !contains(githubtmpl.CompatModes, r.compat) {
		return r, refuse("compat_mode")
	}
	if r.timeout, ok = timeoutOption(a.TimeoutMs); !ok {
		return r, refuse("timeout.over_360")
	}
	sum := sha256.Sum256(a.Spec)
	r.digest = "sha256:" + hex.EncodeToString(sum[:])
	if a.SpecDigest != "" && a.SpecDigest != r.digest {
		return r, refuse("spec.digest")
	}
	if r.spec, err = encodeSpec(a.Spec); err != nil {
		return r, err
	}
	r.trusted = trustedClass(a.Trust)
	return r, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// dispatchRef is the ref the workflow runs from: the default branch for trusted work, the operator's
// untrusted ref for everything else.
func (p *Provider) dispatchRef(ctx context.Context, c *rest.Client, r request, s Settings) (string, error) {
	if !r.trusted {
		if !ValidRef(s.UntrustedRef) {
			return "", misconfig("untrusted_ref")
		}
		return s.UntrustedRef, nil
	}
	var info repoInfo
	if _, err := c.Do(ctx, "GET", repoPath, r.repo.params(), nil, &info); err != nil {
		return "", err
	}
	if !ValidRef(info.DefaultBranch) {
		return "", misconfig("default_branch")
	}
	return info.DefaultBranch, nil
}

type dispatchBody struct {
	Ref              string            `json:"ref"`
	Inputs           map[string]string `json:"inputs"`
	ReturnRunDetails bool              `json:"return_run_details"`
}

func (r request) body(ref, nonce string) dispatchBody {
	in := map[string]string{"spec": r.spec, "spec_digest": r.digest, "revision": r.revision, "attempt": r.attempt, "nonce": nonce,
		"runner_label": r.label, "compat_mode": r.compat, "timeout_minutes": r.timeout}
	if r.fetchRef != "" {
		in["fetch_ref"] = r.fetchRef
	}
	return dispatchBody{Ref: ref, Inputs: in, ReturnRunDetails: true}
}

// Dispatch implements providers.Dispatcher.
func (p *Provider) Dispatch(ctx context.Context, a providers.Attempt) (providers.Handle, error) {
	if !compat.V15() { // compat.V15(P7-HOST-03): no hosted dispatch -> GitHub-hosted workflow dispatch
		return providers.Handle{}, refuse("hosted dispatch needs v1.5")
	}
	r, err := plan(a)
	if err != nil {
		return providers.Handle{}, err
	}
	s, err := p.settings(ctx)
	if err != nil {
		return providers.Handle{}, failure(providers.Config, "E700", "provider policy unreadable")
	}
	c, err := p.client(ctx)
	if err != nil {
		return providers.Handle{}, err
	}
	ref, err := p.dispatchRef(ctx, c, r, s)
	if err != nil {
		return providers.Handle{}, err
	}
	fresh, err := p.cfg.Nonce()
	if err != nil {
		return providers.Handle{}, err
	}
	start := p.cfg.Clock()
	stand, previous, err := p.jrnl().Begin(ctx, Record{Attempt: a.ID, N: a.N, Revision: a.Revision, SpecDigest: r.digest, Nonce: fresh, Ref: ref})
	if err != nil {
		if errors.Is(err, errSuperseded) {
			return providers.Handle{}, refuse("attempt.superseded")
		}
		return providers.Handle{}, failure(providers.Transient, "E704", "journal")
	}
	if previous != nil { // a retry never leaves the previous attempt's run running
		p.cancelNonce(ctx, c, r.repo, previous.Nonce, start)
	}
	h := providers.Handle{Provider: p.ID(), Nonce: stand.Nonce, Ref: stand.Ref}
	if stand.Nonce != fresh { // this attempt number was begun before: its run may exist already
		if id, found, ferr := p.findRun(ctx, c, r.repo, stand.Nonce, start.Add(-leaseTTL-window), start.Add(window)); ferr != nil {
			return providers.Handle{}, ferr
		} else if found {
			h.RunID = runHandle(r.repo, id)
			return h, nil
		}
	}
	return p.send(ctx, c, r, h, start)
}

// send posts the dispatch once and resolves the run id from the response or by nonce.
func (p *Provider) send(ctx context.Context, c *rest.Client, r request, h providers.Handle, start time.Time) (providers.Handle, error) {
	var out struct {
		WorkflowRunID int64 `json:"workflow_run_id"`
	}
	_, err := c.Do(ctx, "POST", dispatchPath, r.repo.params(), r.body(h.Ref, h.Nonce), &out)
	switch {
	case err == nil && out.WorkflowRunID > 0:
		h.RunID = runHandle(r.repo, out.WorkflowRunID)
		return h, nil
	case err != nil && !transient(err):
		return providers.Handle{}, err
	}
	id, found, ferr := p.awaitRun(ctx, c, r.repo, h.Nonce, start)
	switch {
	case found:
		h.RunID = runHandle(r.repo, id)
		return h, nil
	case ferr != nil:
		return providers.Handle{}, ferr
	case err != nil:
		return providers.Handle{}, err
	}
	return providers.Handle{}, failure(providers.Transient, "E704", "run.not_found")
}
