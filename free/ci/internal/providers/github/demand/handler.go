package demand

// Purpose: Handler, the providers.DemandHandler for source github (Epic D6, review F4, F5, F8). It decides
// whether a queued workflow_job may become a runner job on nSelf hardware and, if so, builds the
// synthetic pipeline through runnerjob.Synthesize.
// Inputs:  model.TriggerFacts of kind runner_demand (RunnerLabels, ProviderJobID, Repo) from the webhook
//          source (P7-HOST-05) or the demand poll (P7-HOST-17).
// Outputs: a *model.Pipeline, or a *providers.Error: E708 label.not_allowed (a refusal: the job is never
//          offered again), E703 runner.demand_capped (not a refusal: the poll offers it again when a slot
//          frees), E704 for a transient miss (no ledger yet, policy unreadable), E700 for a bad pin.
// Constraints: the label set is fixed: every label must be one of self-hosted, the operator's jit label,
//          linux, x64 or arm64 (case-insensitive) and the set must contain self-hosted and the jit label,
//          so a fork's workflow that asks for ubuntu-latest, windows or macos is refused and never pulls
//          a hosted job onto an nSelf runner. The labels are read, never copied: the registration uses its
//          own fixed labels (jit.MintRunner). Nothing else a demand carries (ref, actor, head branch,
//          workflow) reaches the pipeline. Trust is not decided here: TRUST-13 classifies the same facts
//          at dispatch, and a repo-scope runner is always untrusted.

import (
	"context"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
)

// Source is the demand source this handler serves.
const Source = "github"

// CredentialKind is the lease credential kind of a GitHub JIT config.
const CredentialKind = "github-jitconfig"

// runnerScript starts the one runner of the image under the hardened profile. The stock run.sh cannot: the
// profile's root file system is read-only, /tmp is noexec and the runner writes next to its own binary
// (_diag, the .runner, .credentials and .credentials_rsaparams files, run-helper.sh). The agent gives a
// runner job an exec-capable tmpfs at /runner (memory only), so the script copies bin/ (88 MB) there, links
// the read-only externals/ and the workspace's _work, and runs Runner.Listener from /runner. The decoded
// credential files the runner writes therefore live in memory, never on the node's disk. The runner reads
// the one-job registration from ACTIONS_RUNNER_INPUT_JITCONFIG, which cred-wrapper set in this shell's
// environment; the script never names it.
const runnerScript = `set -e; r=/runner; mkdir -p /repo/_work; cp -a /home/runner/bin "$r/bin"; ` +
	`ln -s /home/runner/externals "$r/externals"; ln -s /repo/_work "$r/_work"; exec "$r/bin/Runner.Listener" run`

// RunnerCommand is the job command of a GitHub runner job.
var RunnerCommand = []string{"sh", "-c", runnerScript}

// Handler implements providers.DemandHandler. The zero value uses the process-wide policy source, the
// pinned image row and the bound ledger.
type Handler struct {
	Settings func(context.Context) (Settings, error) // default CurrentSettings
	Image    func() (string, error)                  // default PinnedImage
	Ledger   func() *runnerjob.Ledger                // default runnerjob.CurrentLedger
}

var _ providers.DemandHandler = (*Handler)(nil)

func refuse(code, reason string) error {
	return providers.NewError(providers.Config, code, reason, 0, time.Time{})
}

func again(class providers.ErrorClass, code, reason string) error {
	return providers.NewError(class, code, reason, 0, time.Time{})
}

// archOf picks the runner architecture from the allowed labels: arm64 only when x64 is not also asked.
func archOf(labels []string) string {
	arm, x64 := false, false
	for _, l := range labels {
		switch strings.ToLower(l) {
		case "arm64":
			arm = true
		case "x64":
			x64 = true
		}
	}
	if arm && !x64 {
		return "arm64"
	}
	return "amd64"
}

// ascii reports whether every byte of s is below 0x80.
func ascii(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// labelsAllowed applies the fixed label set.
func labelsAllowed(labels []string, jitLabel string) bool {
	var self, mine bool
	for _, l := range labels {
		if !ascii(l) { // strings.ToLower folds "lİnux" to "linux", which GitHub's own match does not
			return false
		}
		switch l = strings.ToLower(l); l {
		case "self-hosted":
			self = true
		case strings.ToLower(jitLabel):
			mine = true
		case "linux", "x64", "arm64":
		default:
			return false
		}
	}
	return self && mine
}

// Synthesize implements providers.DemandHandler.
func (h *Handler) Synthesize(ctx context.Context, f model.TriggerFacts) (*model.Pipeline, error) {
	// compat.V15(P7-HOST-19): no forge job becomes a runner job (v1.4) -> a queued workflow_job becomes a provider-runner pipeline
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
	labels := *f.RunnerLabels.Value
	if !labelsAllowed(labels, s.Label) {
		return nil, refuse("E708", "label.not_allowed")
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
		Credential: CredentialKind, Arch: archOf(labels), Command: RunnerCommand}
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
