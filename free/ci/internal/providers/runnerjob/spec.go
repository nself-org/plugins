// Package runnerjob is the launch half of Epic HOST D6 (P7-HOST-19): it turns a forge's demand for one
// runner into a synthetic single-job pipeline, mints the runner credential only after admission, hands
// it to the job on stdin through the agent's credential wrapper, and deregisters the runner when the run
// closes or an orphan sweep finds it. It is provider-neutral; a provider adds a Minter (GitHub: the
// github/demand package over the JIT registrar, GitLab: P7-HOST-07).
package runnerjob

// Purpose: Synthesize, the one place that decides what a forge-runner job looks like (Epic D6, review F4-F8).
// Inputs:  a Spec naming the provider, repository, demand id, the pinned runner image and credential kind.
// Outputs: a *model.Pipeline (kind adhoc, name provider-runner, one job) whose job runs the runner image
//          for exactly one forge job: container isolation sandboxed-container, no secrets, not idempotent,
//          no infra retry, remote only, the credential kind set so the transport preparer mints it.
// Constraints: RunnerCredential and RemoteOnly are `json:"-"` fields of model.Job, so they do not survive
//          the intake store; Restore puts them back for a pipeline this package built. The network scope
//          is not a job field: an untrusted attempt runs under the restricted scope and the agent hook of
//          this package adds the provider's host allowlist for it (agent.go). The band is not a pipeline
//          field either: BandOf names it for the dispatcher. Nothing a demand carried (labels, ref, actor)
//          reaches the job.

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/background"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Names and environment keys of a provider-runner pipeline.
const (
	PipelineName = "provider-runner"
	// Band is the priority band the pipeline declares (Epic D6: runner capacity is background work).
	Band = background.BandName
	// EnvProvider, EnvRepo and EnvDemand tell the transport preparer which registration to mint. They
	// hold public facts (the forge, the repository and the forge's own job id), never a credential.
	EnvProvider = "NSELF_RUNNERJOB_PROVIDER"
	EnvRepo     = "NSELF_RUNNERJOB_REPO"
	EnvDemand   = "NSELF_RUNNERJOB_DEMAND"
	// EnvHosts carries the extra hosts the runner may reach under the restricted network scope: the
	// operator's own forge host (a self-hosted GitLab), comma separated. Public facts, never a credential.
	EnvHosts = "NSELF_RUNNERJOB_HOSTS"
	// MaxHosts bounds Spec.Hosts.
	MaxHosts = 4
	// Isolation is the floor a forge-runner job runs under; below it a fork's code would share a kernel.
	Isolation = model.Isolation("sandboxed-container")
	// JobTimeout bounds one runner job (the forge's own job limit is six hours).
	JobTimeout = "6h"
)

var (
	repoPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}/[A-Za-z0-9._-]{1,100}$`)
	demandPattern = regexp.MustCompile(`^[0-9]{1,20}$`)
	imagePattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9./_:-]*@sha256:[0-9a-f]{64}$`)
	kindPattern   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,40}$`)
	hostPattern   = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)
)

// Spec is what Synthesize needs to describe one runner job.
type Spec struct {
	Provider   string   // "github"
	Repo       string   // "owner/name"
	Demand     string   // the forge's job id, digits
	Image      string   // the pinned runner image, repository[:tag]@sha256:<64 hex>
	Credential string   // lease credential kind, for example github-jitconfig
	Arch       string   // amd64 or arm64
	Command    []string // how the image starts one runner, for example /home/runner/run.sh
	Hosts      []string // lower-case host names the runner may reach besides the provider's fixed list (self-hosted forge)
}

// Validate refuses a spec that could not describe a safe job.
func (s Spec) Validate() error {
	switch {
	case !kindPattern.MatchString(s.Provider):
		return errors.New("runnerjob: provider is not a plain word")
	case !repoPattern.MatchString(s.Repo):
		return errors.New("runnerjob: repo is not owner/name")
	case !demandPattern.MatchString(s.Demand):
		return errors.New("runnerjob: demand id is not a number")
	case !imagePattern.MatchString(s.Image):
		return errors.New("runnerjob: the image must be pinned by sha256 digest")
	case !kindPattern.MatchString(s.Credential):
		return errors.New("runnerjob: credential kind is not a plain word")
	case s.Arch != "amd64" && s.Arch != "arm64":
		return errors.New("runnerjob: only linux amd64 and arm64 runners are launched")
	case len(s.Command) == 0 || strings.TrimSpace(s.Command[0]) == "":
		return errors.New("runnerjob: the runner start command is empty")
	case len(s.Hosts) > MaxHosts:
		return errors.New("runnerjob: too many extra hosts")
	}
	for _, h := range s.Hosts {
		if !hostPattern.MatchString(h) {
			return errors.New("runnerjob: an extra host is not a plain lower-case host name")
		}
	}
	return nil
}

func ptr[T any](v T) *T { return &v }

// Synthesize builds the pipeline of one forge-runner job.
func Synthesize(s Spec) (*model.Pipeline, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	job := model.Job{
		Kind:       "build",
		Run:        append([]string(nil), s.Command...),
		Idempotent: ptr(false),
		Required:   ptr(false),
		Timeout:    JobTimeout,
		Retry:      &model.RetryConfig{InfraMax: 0},
		Isolation:  Isolation,
		Container:  &model.ContainerConfig{Image: s.Image},
		Matrix:     &model.JobMatrix{Platform: "linux/" + s.Arch},
		Env: map[string]string{
			EnvProvider: s.Provider,
			EnvRepo:     s.Repo,
			EnvDemand:   s.Demand,
		},
		Secrets:          nil,
		RunnerCredential: s.Credential,
		RemoteOnly:       true,
	}
	if len(s.Hosts) > 0 {
		job.Env[EnvHosts] = strings.Join(s.Hosts, ",")
	}
	return &model.Pipeline{
		ID:        fmt.Sprintf("%s-%s-%s", PipelineName, s.Provider, s.Demand),
		Name:      PipelineName,
		Kind:      "adhoc",
		Selection: model.Selection{Mode: "full", Reason: "selection.full_required"},
		Jobs:      []model.Job{job},
	}, nil
}

// IsRunnerPipeline reports whether p is a pipeline Synthesize built (by shape, not by name alone: the
// job must carry the three markers and exactly one job).
func IsRunnerPipeline(p *model.Pipeline) bool {
	if p == nil || p.Name != PipelineName || p.Kind != "adhoc" || len(p.Jobs) != 1 {
		return false
	}
	env := p.Jobs[0].Env
	return env[EnvProvider] != "" && env[EnvRepo] != "" && env[EnvDemand] != ""
}

// BandOf is the band the dispatcher gives p: background for a runner pipeline, "" for any other.
func BandOf(p *model.Pipeline) string {
	if IsRunnerPipeline(p) {
		return Band
	}
	return ""
}

// Restore puts back the two fields JSON drops (RemoteOnly and the credential kind) on the job of a
// runner pipeline read from the intake store. The kind comes from kindOf(provider); a provider with no
// kind leaves the job without a credential, which the preparer then refuses to run as a runner.
func Restore(p *model.Pipeline, kindOf func(provider string) string) {
	if !IsRunnerPipeline(p) {
		return
	}
	j := &p.Jobs[0]
	j.RemoteOnly = true
	if kindOf != nil {
		j.RunnerCredential = kindOf(j.Env[EnvProvider])
	}
}
