package adhoc

// Purpose: stamp the ad-hoc facts on the run evidence before the evidence
// builder (P7-CI-38) derives the per-attempt documents from it.
// Inputs:  the base evidence document, the Mode, the redacted argv.
// Outputs: the document with runner.isolation set to the level the registry served,
// the requested trust class, the redacted argv as the check's tool and the
// network scope (and image) under tool_versions; then the registered builder's
// result.
// Constraints: the evidence schema has no network field and forbids extra
// properties, so the scope rides in tool_versions (a free string map); the
// pipeline kind `adhoc` comes from the builder (the run is a one-job run), and
// every consumer that gates (evidence verify, attest) already rejects it.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// Tool-version keys the ad-hoc job records.
const (
	KeyNetwork = "adhoc.network_scope"
	KeyImage   = "adhoc.image"
)

// evidenceStamp implements run.EvidenceBuilder around the registered builder.
type evidenceStamp struct {
	inner cirun.EvidenceBuilder
	mode  Mode
	argv  string // redacted
	image string
}

func (e evidenceStamp) Begin(ctx context.Context, root string, rev cirun.Revision, git cirun.Capture) any {
	if e.inner == nil {
		return nil
	}
	return e.inner.Begin(ctx, root, rev, git)
}

func (e evidenceStamp) Complete(ctx context.Context, begin any, f cirun.EvidenceFacts, ev model.Evidence) (model.Evidence, error) {
	ev = e.Stamp(ev)
	if e.inner == nil {
		return ev, nil
	}
	out, err := e.inner.Complete(ctx, begin, f, ev)
	if err != nil {
		return ev, err // the coordinator retains the stamped base, without input digests
	}
	return out, nil
}

// Stamp preserves ad-hoc facts even when the inner evidence builder fails.
func (e evidenceStamp) Stamp(ev model.Evidence) model.Evidence {
	ev.Runner.Isolation = e.mode.Served // what the registry served, never the level asked for (D-1021)
	if ev.Runner.Isolation == "" {
		ev.Runner.Isolation = e.mode.Level
	}
	if e.mode.Trust != "" {
		ev.SourceTrust = e.mode.Trust
	}
	tv := make(map[string]string, len(ev.ToolVersions)+2)
	for k, v := range ev.ToolVersions {
		tv[k] = v
	}
	tv[KeyNetwork] = string(e.mode.Network)
	if e.image != "" {
		tv[KeyImage] = e.image
	}
	ev.ToolVersions = tv
	checks := append([]model.Check(nil), ev.Checks...)
	for i := range checks {
		checks[i].Tool = e.argv
	}
	ev.Checks = checks
	return ev
}
