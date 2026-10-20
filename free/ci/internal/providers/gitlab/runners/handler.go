package runners

// Purpose: Handler, the providers.DemandHandler for source gitlab (Epic D11, review F4, F5, F8). It decides
// whether a pending job with the nself tag may become a runner job on nSelf hardware and, if so, builds the
// synthetic pipeline through runnerjob.Synthesize.
// Inputs:  model.TriggerFacts of kind runner_demand (RunnerLabels, ProviderJobID, Repo) from the GitLab
//          Job Hook source (P7-HOST-21).
// Outputs: a *model.Pipeline, or a *providers.Error: E708 label.not_allowed (a refusal: the job is never
//          offered again), E703 runner.demand_capped (not a refusal: the job is offered again when a slot
//          frees), E704 for a transient miss (no ledger yet, policy unreadable), E700 for a bad pin or a
//          bad instance URL.
// Constraints: the tag set is fixed: every tag of the demand must be one of providers.gitlab.runner.tags
//          (exact, ASCII) and the demand must carry the nself tag, so a job that asks for a tag the
//          operator did not configure is refused and never pulls other work onto an nSelf runner. The tags
//          are read, never copied: the registration uses its own fixed tag list (Registrar.MintRunner).
//          Nothing else a demand carries (ref, user, branch, variables) reaches the pipeline. Trust is not
//          decided here: TRUST-13 classifies the same facts at dispatch, and a project runner is always
//          untrusted.

import (
	"context"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
)

// Source is the demand source this handler serves.
const Source = "gitlab"

// Handler implements providers.DemandHandler. The zero value uses the process-wide policy source, the
// pinned image row and the bound ledger.
type Handler struct {
	Settings func(context.Context) (Settings, error)           // default CurrentSettings
	Instance func(context.Context) (url, host string, e error) // default InstanceFromPolicy
	Image    func() (string, error)                            // default PinnedImage
	Ledger   func() *runnerjob.Ledger                          // default runnerjob.CurrentLedger
}

var _ providers.DemandHandler = (*Handler)(nil)

func refuse(code, reason string) error {
	return providers.NewError(providers.Config, code, reason, 0, time.Time{})
}

func again(class providers.ErrorClass, code, reason string) error {
	return providers.NewError(class, code, reason, 0, time.Time{})
}

// InstanceFromPolicy reads the GitLab instance URL and host from providers.gitlab.api_url.
func InstanceFromPolicy(ctx context.Context) (instance, host string, err error) {
	rs, err := runtime.CurrentSettings(ctx)
	if err != nil {
		return "", "", err
	}
	instance, host, ok := InstanceOf(rs.GitLabAPIURL)
	if !ok {
		return "", "", refuse("E700", "runner.instance_url")
	}
	return instance, host, nil
}

func ascii(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// tagsAllowed applies the fixed tag set: a non-empty demand, every tag configured, the nself tag present.
func tagsAllowed(demand, configured []string) bool {
	allowed := make(map[string]bool, len(configured))
	for _, t := range configured {
		allowed[t] = true
	}
	mine := false
	for _, t := range demand {
		if !ascii(t) || !allowed[t] {
			return false
		}
		mine = mine || t == DemandTag
	}
	return mine
}

// archOf picks the runner architecture: arm64 only when the demand asks for it and it is a configured tag.
func archOf(demand []string) string {
	for _, t := range demand {
		if strings.EqualFold(t, "arm64") {
			return "arm64"
		}
	}
	return "amd64"
}

// Synthesize implements providers.DemandHandler.
func (h *Handler) Synthesize(ctx context.Context, f model.TriggerFacts) (*model.Pipeline, error) {
	// compat.V15(P7-HOST-07): no forge job becomes a runner job (v1.4) -> a pending GitLab job becomes a provider-runner pipeline
	if !compat.V15() {
		return nil, refuse("E700", "compat.v14")
	}
	if f.Source != Source || f.Kind != "runner_demand" {
		return nil, refuse("E702", "demand.kind")
	}
	if f.RunnerLabels.Value == nil || f.ProviderJobID.Value == nil || f.Repo == "" {
		return nil, refuse("E702", "demand.facts")
	}
	settings := h.Settings
	if settings == nil {
		settings = CurrentSettings
	}
	s, err := settings(ctx)
	if err != nil {
		return nil, again(providers.Transient, "E704", "demand.settings")
	}
	tags := *f.RunnerLabels.Value
	if !tagsAllowed(tags, s.Tags) {
		return nil, refuse("E708", "label.not_allowed")
	}
	inst := h.Instance
	if inst == nil {
		inst = InstanceFromPolicy
	}
	instance, host, err := inst(ctx)
	if err != nil {
		if _, isProv := err.(*providers.Error); isProv {
			return nil, err
		}
		return nil, again(providers.Transient, "E704", "demand.settings")
	}
	image := h.Image
	if image == nil {
		image = PinnedImage
	}
	ref, err := image()
	if err != nil {
		return nil, refuse("E700", "runner.image_pin")
	}
	spec := runnerjob.Spec{Provider: Source, Repo: f.Repo, Demand: *f.ProviderJobID.Value, Image: ref,
		Credential: CredentialKind, Arch: archOf(tags), Command: RunnerCommand(instance), Hosts: []string{host}}
	if err = spec.Validate(); err != nil {
		return nil, refuse("E702", "demand.facts")
	}
	ledger := runnerjob.CurrentLedger
	if h.Ledger != nil {
		ledger = h.Ledger
	}
	led := ledger()
	if led == nil {
		return nil, again(providers.Transient, "E704", "runner.not_ready")
	}
	ok, err := led.Reserve(ctx, Source, f.Repo, spec.Demand, s.MaxPending)
	switch {
	case err != nil:
		return nil, again(providers.Transient, "E704", "runner.ledger")
	case !ok:
		return nil, again(providers.Quota, "E703", "runner.demand_capped")
	}
	return runnerjob.Synthesize(spec)
}
