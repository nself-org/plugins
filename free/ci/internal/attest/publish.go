package attest

// Purpose: Publish applies the publication profile to an evidence document so
// it can leave the machine for a public repository: runner ids become stable
// pseudonyms, and private addresses, host names, user names, home paths and
// private URLs disappear from every free-text field.
// Inputs:  the evidence and the repository visibility.
// Outputs: a copy of the evidence; the input is never modified.
// Constraints: the profile knows the runner id and the host names and user
// names the caller gives it (PublishWith); a host name it was never told and
// that carries no private suffix can survive in a log excerpt, which is why a
// host passes its own host and user names. A private repository's evidence is returned unchanged; any
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
	// structured fields use the plain profile; the runner id is also replaced
	// inside free text, where a host name tends to surface
	structured, p := p, p.withRunner(ev.Runner.ID)
	out := ev
	out.Runner.ID = p.RunnerID(ev.Runner.ID)
	out.Runner.Provider = structured.Text(ev.Runner.Provider)
	out.Runner.Platform = structured.Text(ev.Runner.Platform)
	out.Pipeline.Name = structured.Text(ev.Pipeline.Name)
	out.PlacementRef = scrubPtr(p, ev.PlacementRef)
	// every other free string is scrubbed too: a hostile or careless producer can
	// put a path or an address in any of them
	out.RunID = p.Text(ev.RunID)
	out.Pipeline.ID = p.Text(ev.Pipeline.ID)
	out.Job = scrubPtr(p, ev.Job)
	if ev.Attempt != nil {
		a := *ev.Attempt
		a.ID = p.Text(a.ID)
		out.Attempt = &a
	}
	out.Producer.Version = p.Text(ev.Producer.Version)
	if ev.ProviderAttestation != nil {
		pa := *ev.ProviderAttestation
		pa.Provider, pa.RunID, pa.Ref = p.Text(pa.Provider), p.Text(pa.RunID), p.Text(pa.Ref)
		pa.WorkflowPath, pa.TemplateVersion, pa.ArtifactID = p.Text(pa.WorkflowPath), p.Text(pa.TemplateVersion), p.Text(pa.ArtifactID)
		out.ProviderAttestation = &pa
	}
	out.InputDigests = model.InputDigests{Config: ev.InputDigests.Config,
		Lockfiles: scrubRefs(p, ev.InputDigests.Lockfiles), Generated: scrubRefs(p, ev.InputDigests.Generated)}
	out.ToolVersions = make(map[string]string, len(ev.ToolVersions))
	for k, v := range ev.ToolVersions {
		out.ToolVersions[p.Text(k)] = p.Text(v)
	}
	out.Checks = make([]model.Check, len(ev.Checks))
	for i, c := range ev.Checks {
		c.ID, c.JobID, c.Kind = p.Text(c.ID), p.Text(c.JobID), p.Text(c.Kind)
		c.Reason = model.Reason(p.Text(string(c.Reason)))
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
	out.Retries = make([]model.Retry, len(ev.Retries))
	for i, r := range ev.Retries {
		r.JobID = p.Text(r.JobID)
		out.Retries[i] = r
	}
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
