package pipeline

// Purpose: Collect (Epic D11): read what the finished pipeline proves, download its evidence artifact
//          and hand both to hostedev.Verify.
// Inputs:  the handle; the journal record of its nonce (attempt, number, revision, spec digest, ref,
//          mode); GET pipeline, GET the pipeline's user, jobs and variables, the include file raw at
//          the pipeline's sha, the merged CI configuration at that sha, and the nself-ci job's
//          artifact archive.
// Outputs: providers.Collected{Evidence, Attestation} or an E705 whose reason is one hostedev key.
// Constraints: no ref is resolved; the include file is read at the pipeline's own sha, not at the
//          revision under test, and compared byte for byte with the coordinator's rendering by
//          hostedev, then checkConfig binds the rest of the configuration (config.go: the byte-exact
//          generated root for untrusted work, the merged configuration for trusted work); the hostedev run is mapped from GitLab like this: Event is the pipeline source
//          (must be "api"), Path is the include path and is claimed only when the pipeline holds
//          exactly one job named nself-ci, HeadBranch and HeadSHA are the pipeline's ref and sha,
//          DisplayTitle is the pipeline name (inputs mode) or the echoed NSELF_CI_NONCE variable
//          (variables mode, so a missing echo is run.nonce), RunAttempt is the number of nself-ci jobs
//          including retried ones (a retried job is run.rerun); in variables mode the echoed spec
//          digest must equal the journalled one (spec.digest); the pipeline's user must be the token's
//          user (run.actor); an archive larger than hostedev.MaxArtifactBytes is never downloaded; a
//          handle whose nonce the journal does not know is refused (intent.unknown).

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
	"github.com/nself-org/plugins/free/ci/internal/providers/hostedev"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	gitlabtmpl "github.com/nself-org/plugins/free/ci/templates/gitlab"
)

// The E705 reasons Collect adds to hostedev's.
const (
	ReasonIntentUnknown = "intent.unknown"
	ReasonSuperseded    = "attempt.superseded"
	ReasonActor         = "run.actor"
)

// pipelineSource is the pipeline source GitLab reports for a pipeline created through the API.
const pipelineSource = "api"

func finished(status string) bool {
	switch status {
	case "success", "failed", "canceled", "skipped":
		return true
	}
	return false
}

// Collect implements providers.Dispatcher.
func (p *Provider) Collect(ctx context.Context, h providers.Handle) (providers.Collected, error) {
	project, id, err := p.handleOf(h)
	if err != nil {
		return providers.Collected{}, err
	}
	rec, ok, err := p.jrnl().ByNonce(ctx, h.Nonce)
	if err != nil {
		return providers.Collected{}, api.Unavailable("journal")
	}
	if !ok || rec.Project != project.Path { // a handle never points Collect at another project
		return providers.Collected{}, api.NotEvidence(ReasonIntentUnknown)
	}
	if latest, found, lerr := p.jrnl().Latest(ctx, rec.Lineage); lerr != nil || !found || latest.Nonce != h.Nonce {
		if lerr != nil {
			return providers.Collected{}, api.Unavailable("journal")
		}
		return providers.Collected{}, api.NotEvidence(ReasonSuperseded) // a newer attempt of this job replaced it
	}
	c, err := p.client(ctx)
	if err != nil {
		return providers.Collected{}, err
	}
	var pl api.Pipeline
	if _, err = c.Do(ctx, "GET", api.PipelinePath, project.With("pipeline", intToString(id)), nil, &pl); err != nil {
		return providers.Collected{}, err
	}
	if !finished(pl.Status) {
		return providers.Collected{}, api.Unavailable("run.incomplete")
	}
	login, err := p.whoami(ctx, c)
	if err != nil {
		return providers.Collected{}, err
	}
	if pl.User.Username != login { // a pipeline somebody else created is never ours
		return providers.Collected{}, api.NotEvidence(ReasonActor)
	}
	job, attempts, err := p.nselfJob(ctx, c, project, id)
	if err != nil {
		return providers.Collected{}, err
	}
	title, echoed := pl.Name, ""
	if rec.Mode == ModeVariables {
		vars, verr := p.variables(ctx, c, project, id)
		if verr != nil {
			return providers.Collected{}, verr
		}
		title, echoed = vars[gitlabtmpl.VarNonce], vars[gitlabtmpl.VarDigest]
	}
	file, err := p.includeAt(ctx, c, project, pl.SHA)
	if err != nil {
		return providers.Collected{}, err
	}
	size, zipBytes, err := p.fetchArchive(ctx, c, project, job)
	if err != nil {
		return providers.Collected{}, err
	}
	path := ""
	if attempts > 0 {
		path = gitlabtmpl.Path
	}
	in := hostedev.VerifyInput{
		Provider:      p.ID(),
		Run:           hostedev.Run{ID: h.RunID, Event: pl.Source, Path: path, HeadBranch: pl.Ref, HeadSHA: pl.SHA, DisplayTitle: title, RunAttempt: attempts},
		Want:          hostedev.Want{Event: pipelineSource, Path: gitlabtmpl.Path, Ref: rec.Ref},
		WorkflowBytes: file,
		Rendering: func(version int) ([]byte, error) {
			return p.rendering(ctx, version, file)
		},
		MinTemplate: gitlabtmpl.Minimum, Nonce: h.Nonce, AttemptID: rec.Attempt, AttemptN: rec.N, Revision: rec.Revision,
		SpecDigest: rec.SpecDigest, MinAgent: p.cfg.MinAgent,
		ArtifactID: intToString(job.ID), ArtifactSize: size, EvidenceZip: zipBytes, Now: p.cfg.Clock(),
	}
	att, err := hostedev.Verify(ctx, in)
	if err != nil {
		return providers.Collected{}, err
	}
	if rec.Mode == ModeVariables && echoed != rec.SpecDigest { // the pipeline ran with another spec
		return providers.Collected{}, api.NotEvidence(hostedev.ReasonSpec)
	}
	if err = p.checkConfig(ctx, c, project, pl.SHA, file, rec); err != nil { // nothing but the generated job ran
		return providers.Collected{}, err
	}
	doc, err := hostedev.ReadEvidence(zipBytes)
	if err != nil {
		return providers.Collected{}, err
	}
	return providers.Collected{Evidence: doc, Attestation: att}, nil
}

func (p *Provider) rendering(ctx context.Context, version int, file []byte) ([]byte, error) {
	if p.cfg.Rendering != nil {
		return p.cfg.Rendering(ctx, version, file)
	}
	return DefaultRendering(ctx, version, file)
}

// nselfJob returns the pipeline's one nself-ci job and the number of such jobs including retried ones.
// With none the job is the zero Job and the count is 0 (hostedev then says run.path).
func (p *Provider) nselfJob(ctx context.Context, c *rest.Client, project api.Project, pipeline int64) (api.Job, int, error) {
	jobs, err := api.List[api.Job](ctx, c, api.PipelineJobs, project.With("pipeline", intToString(pipeline)), url.Values{"include_retried": {"true"}})
	if err != nil {
		return api.Job{}, 0, err
	}
	var mine []api.Job
	for _, j := range jobs {
		if j.Name == gitlabtmpl.Job {
			mine = append(mine, j)
		}
	}
	if len(mine) == 0 {
		return api.Job{}, 0, nil
	}
	return mine[0], len(mine), nil // the listing is newest first; a retry makes the count 2 and is refused
}

// includeAt reads the include file as the project has it at sha.
func (p *Provider) includeAt(ctx context.Context, c *rest.Client, project api.Project, sha string) ([]byte, error) {
	if sha == "" {
		return nil, api.NotEvidence(hostedev.ReasonTemplate)
	}
	raw, err := p.fetch(ctx, c, api.RawFileURL(c.BaseURL, project, gitlabtmpl.Path, sha), 1<<20)
	if err != nil {
		if api.IsStatus(err, 404) {
			return nil, api.NotEvidence(hostedev.ReasonTemplate)
		}
		return nil, err
	}
	return raw, nil
}

// fetchArchive downloads the job's artifact archive unless it is missing, expired or too large; a
// missing archive is size 0 and a nil zip (hostedev then says artifact.missing).
func (p *Provider) fetchArchive(ctx context.Context, c *rest.Client, project api.Project, job api.Job) (int64, []byte, error) {
	size, ok := job.ArchiveSize()
	if job.ID == 0 || !ok || job.ArtifactsExpireAt != nil && job.ArtifactsExpireAt.Before(p.cfg.Clock()) {
		return 0, nil, nil
	}
	if size > hostedev.MaxArtifactBytes {
		return size, nil, nil
	}
	data, err := p.fetch(ctx, c, api.ArtifactsURL(c.BaseURL, project, job.ID), hostedev.MaxArtifactBytes)
	if err != nil {
		if pe, isPE := err.(*providers.Error); isPE && strings.HasSuffix(pe.Reason, "too_large") {
			return size, nil, api.NotEvidence(hostedev.ReasonTooLarge)
		}
		if api.IsStatus(err, 404) {
			return 0, nil, nil // expired between the listing and the download
		}
		return 0, nil, err
	}
	return size, data, nil
}

func (p *Provider) fetch(ctx context.Context, c *rest.Client, link string, max int64) ([]byte, error) {
	var buf bytes.Buffer
	download := p.cfg.Download
	if download == nil {
		download = func(ctx context.Context, c *rest.Client, link string, max int64, w io.Writer) error {
			_, err := c.Download(ctx, link, max, w)
			return err
		}
	}
	if err := download(ctx, c, link, max, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
