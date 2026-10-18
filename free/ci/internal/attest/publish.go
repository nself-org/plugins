package attest

// Purpose: Publish applies the publication profile to an evidence document so
// it can leave the machine for a public repository: runner ids become stable
// pseudonyms, and private addresses, host names, user names, home paths and
// private URLs disappear from every free-text field.
// Inputs:  the evidence and the repository visibility.
// Outputs: a copy of the evidence; the input is never modified.
// Constraints: a private repository's evidence is returned unchanged; any
// visibility other than Private scrubs (fail closed). The copy no longer
// matches a signature made over the original, so it is a report, not a gate
// input: its freshness is marked unknown.

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Publish returns ev as it may be published for a repository of visibility v.
func Publish(ev model.Evidence, v Visibility) model.Evidence {
	return PublishWith(ev, ProfileFor(RepoFacts{Visibility: v, RepoID: ev.RepoID}))
}

// PublishWith is Publish with a caller-built profile (extra hostnames and
// usernames to redact).
func PublishWith(ev model.Evidence, p Profile) model.Evidence {
	if !p.Public() {
		return ev
	}
	p = p.withRunner(ev.Runner.ID)
	out := ev
	out.Runner.ID = p.RunnerID(ev.Runner.ID)
	out.Runner.Provider = p.Text(ev.Runner.Provider)
	out.Runner.Platform = p.Text(ev.Runner.Platform)
	out.Pipeline.Name = p.Text(ev.Pipeline.Name)
	out.PlacementRef = scrubPtr(p, ev.PlacementRef)
	out.InputDigests = model.InputDigests{Config: ev.InputDigests.Config,
		Lockfiles: scrubRefs(p, ev.InputDigests.Lockfiles), Generated: scrubRefs(p, ev.InputDigests.Generated)}
	out.ToolVersions = make(map[string]string, len(ev.ToolVersions))
	for k, v := range ev.ToolVersions {
		out.ToolVersions[k] = p.Text(v)
	}
	out.Checks = make([]model.Check, len(ev.Checks))
	for i, c := range ev.Checks {
		c.Tool = p.Text(c.Tool)
		c.ToolVersion = scrubPtr(p, c.ToolVersion)
		c.Excerpt = p.Text(c.Excerpt)
		c.LogRef = scrubPtr(p, c.LogRef)
		out.Checks[i] = c
	}
	out.Artifacts = make([]model.Artifact, len(ev.Artifacts))
	for i, a := range ev.Artifacts {
		a.Path = p.Text(a.Path)
		out.Artifacts[i] = a
	}
	out.Retries = append([]model.Retry{}, ev.Retries...)
	reason := "publication profile applied; recompute from the inputs"
	out.Freshness = model.Freshness{State: "unknown", Reason: &reason}
	return out
}

func scrubPtr(p Profile, s *string) *string {
	if s == nil {
		return nil
	}
	v := p.Text(*s)
	return &v
}

func scrubRefs(p Profile, refs []model.DigestRef) []model.DigestRef {
	out := make([]model.DigestRef, len(refs))
	for i, r := range refs {
		out[i] = model.DigestRef{Path: p.Text(r.Path), SHA256: r.SHA256}
	}
	return out
}
