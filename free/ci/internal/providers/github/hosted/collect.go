package hosted

// Purpose: Collect (Epic D5): read what the finished run proves, download its evidence artifact and
//          hand both to hostedev.Verify.
// Inputs:  the handle; the journal record of its nonce (attempt, number, revision, spec digest);
//          GET run, GET the workflow file at the run's head_sha, GET the run's artifacts, and the
//          artifact zip through the funnel's allowlisted download.
// Outputs: providers.Collected{Evidence, Attestation} or an E705 whose reason is one hostedev key.
// Constraints: no ref is resolved; the workflow file is read at the run's own commit, not at the
//          revision under test; an artifact larger than hostedev.MaxArtifactBytes is never
//          downloaded; a handle whose nonce the journal does not know is refused (E705
//          intent.unknown), so evidence is only ever accepted for a dispatch this coordinator
//          recorded; the zip is only read through hostedev.

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/hostedev"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	githubtmpl "github.com/nself-org/plugins/free/ci/templates/github"
)

// EvidenceArtifact is the artifact name of one attempt's evidence.
func EvidenceArtifact(nonce string) string { return "nself-ci-evidence-" + nonce }

// ReasonIntentUnknown is the E705 reason for a handle this coordinator never journalled.
const ReasonIntentUnknown = "intent.unknown"

// Collect implements providers.Dispatcher.
func (p *Provider) Collect(ctx context.Context, h providers.Handle) (providers.Collected, error) {
	repo, params, err := p.handleParams(h)
	if err != nil {
		return providers.Collected{}, err
	}
	rec, ok, err := p.jrnl().ByNonce(ctx, h.Nonce)
	if err != nil {
		return providers.Collected{}, failure(providers.Transient, "E704", "journal")
	}
	if !ok {
		return providers.Collected{}, notEvidence(ReasonIntentUnknown)
	}
	c, err := p.client(ctx)
	if err != nil {
		return providers.Collected{}, err
	}
	var run runInfo
	if _, err = c.Do(ctx, "GET", runPath, params, nil, &run); err != nil {
		return providers.Collected{}, err
	}
	if run.Status != "completed" {
		return providers.Collected{}, failure(providers.Transient, "E704", "run.incomplete")
	}
	workflow, err := p.workflowAt(ctx, c, repo, run.HeadSHA)
	if err != nil {
		return providers.Collected{}, err
	}
	art, zipBytes, err := p.fetchArtifact(ctx, c, repo, params, h.Nonce)
	if err != nil {
		return providers.Collected{}, err
	}
	in := hostedev.VerifyInput{
		Provider: p.ID(),
		Run: hostedev.Run{ID: h.RunID, Event: run.Event, Path: strings.SplitN(run.Path, "@", 2)[0], HeadBranch: run.HeadBranch, HeadSHA: run.HeadSHA,
			DisplayTitle: run.DisplayTitle, RunAttempt: run.RunAttempt},
		Want:          hostedev.Want{Event: "workflow_dispatch", Path: WorkflowPath, Ref: h.Ref},
		WorkflowBytes: workflow,
		Rendering: func(version int) ([]byte, error) {
			return p.rendering(ctx, version, workflow)
		},
		MinTemplate: githubtmpl.Minimum, Nonce: h.Nonce, AttemptID: rec.Attempt, AttemptN: rec.N, Revision: rec.Revision,
		SpecDigest: rec.SpecDigest, MinAgent: p.cfg.MinAgent,
		ArtifactID: intToString(art.ID), ArtifactSize: art.Size, EvidenceZip: zipBytes, Now: p.cfg.Clock(),
	}
	att, err := hostedev.Verify(ctx, in)
	if err != nil {
		return providers.Collected{}, err
	}
	doc, err := hostedev.ReadEvidence(zipBytes)
	if err != nil {
		return providers.Collected{}, err
	}
	return providers.Collected{Evidence: doc, Attestation: att}, nil
}

func (p *Provider) rendering(ctx context.Context, version int, workflow []byte) ([]byte, error) {
	if p.cfg.Rendering != nil {
		return p.cfg.Rendering(ctx, version, workflow)
	}
	return DefaultRendering(ctx, version, workflow)
}

// workflowAt reads the workflow file as the repository has it at sha.
func (p *Provider) workflowAt(ctx context.Context, c *rest.Client, repo repoRef, sha string) ([]byte, error) {
	if sha == "" {
		return nil, notEvidence(hostedev.ReasonTemplate)
	}
	var f contentsInfo
	if _, err := c.DoQuery(ctx, "GET", contentsPath, repo.params(), url.Values{"ref": {sha}}, nil, &f); err != nil {
		if isStatus(err, 404) {
			return nil, notEvidence(hostedev.ReasonTemplate)
		}
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.NewReplacer("\n", "", "\r", "").Replace(f.Content))
	if f.Encoding != "base64" || err != nil {
		return nil, notEvidence(hostedev.ReasonTemplate)
	}
	return raw, nil
}

// fetchArtifact finds the attempt's evidence artifact and downloads it unless it is too large; a
// missing artifact is a zero artifactInfo and a nil zip (hostedev then says artifact.missing).
func (p *Provider) fetchArtifact(ctx context.Context, c *rest.Client, repo repoRef, params map[string]string, nonce string) (artifactInfo, []byte, error) {
	var list struct {
		Artifacts []artifactInfo `json:"artifacts"`
	}
	if _, err := c.Do(ctx, "GET", artifactsFor, params, nil, &list); err != nil {
		return artifactInfo{}, nil, err
	}
	var art artifactInfo
	for _, a := range list.Artifacts {
		if a.Name == EvidenceArtifact(nonce) && !a.Expired {
			if art.ID != 0 { // two artifacts of one name: take neither
				return artifactInfo{}, nil, notEvidence(hostedev.ReasonArtifact)
			}
			art = a
		}
	}
	if art.ID == 0 || art.Size > hostedev.MaxArtifactBytes {
		return art, nil, nil
	}
	var buf bytes.Buffer
	link := strings.TrimRight(c.BaseURL, "/") + "/repos/" + repo.String() + "/actions/artifacts/" + strconv.FormatInt(art.ID, 10) + "/zip"
	download := p.cfg.Download
	if download == nil {
		download = func(ctx context.Context, c *rest.Client, link string, max int64, w io.Writer) error {
			_, err := c.Download(ctx, link, max, w)
			return err
		}
	}
	if err := download(ctx, c, link, hostedev.MaxArtifactBytes, &buf); err != nil {
		if pe, ok := err.(*providers.Error); ok && strings.HasSuffix(pe.Reason, "too_large") {
			return art, nil, notEvidence(hostedev.ReasonTooLarge)
		}
		return artifactInfo{}, nil, err
	}
	return art, buf.Bytes(), nil
}
