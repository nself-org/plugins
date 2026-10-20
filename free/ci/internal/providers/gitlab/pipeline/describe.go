package pipeline

// Purpose: Describe (Epic D11): the shared runners GitLab offers for one project, one per tag in
//          templates/gitlab/runner_tags.yaml.
// Inputs:  providers.Query{Project, Repo "group/.../name"}; one GET of the project proves the token
//          can see it.
// Outputs: model.Capability values: identity.provider gitlab, isolation hosted-disposable,
//          secret_classes [none], network internet, economics class included, privacy zone
//          hosted-allowed, no visibility label.
// Constraints: the capability NEVER carries nself.visibility=public (review F19: gitlab.com meters
//          public projects too, so the project's visibility says nothing about free minutes); the
//          allowance stays unknown and the scheduler asks for an owner-declared ceiling; only tags from
//          runner_tags.yaml appear (the Developer token cannot read runner details, and a larger
//          runner would be billed); an empty query describes nothing; a project the token cannot read
//          is the funnel's error, so the contributor lists the provider as skipped; a credential that
//          does not resolve is E700 and reaches the caller.

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
	gitlabtmpl "github.com/nself-org/plugins/free/ci/templates/gitlab"
)

// Label keys of a hosted runner. They are the same strings the GitHub provider uses, because the one
// AttemptFacts source of the transport driver reads them back from the leased node.
const (
	LabelRepo      = "nself.repo="
	LabelProject   = "nself.project="
	LabelRunner    = "nself.runner_label="
	runnerIDPrefix = "gitlab:"
)

// Describe implements providers.Provider.
func (p *Provider) Describe(ctx context.Context, q providers.Query) ([]model.Capability, error) {
	if !compat.V15() { // compat.V15(P7-HOST-06): no hosted capacity -> GitLab shared runners
		return nil, nil
	}
	if q.Repo == "" {
		return nil, nil
	}
	project, ok := api.ParseProject(q.Repo)
	if !ok {
		return nil, api.Misconfig("repo")
	}
	tags, err := gitlabtmpl.Tags()
	if err != nil {
		return nil, api.Misconfig("runner_tags")
	}
	c, err := p.client(ctx)
	if err != nil {
		return nil, err
	}
	var info api.ProjectInfo
	if _, err = c.Do(ctx, "GET", api.ProjectPath, project.Params(), nil, &info); err != nil {
		return nil, err
	}
	now := p.cfg.Clock().UTC().Truncate(time.Second)
	out := make([]model.Capability, 0, len(tags))
	for _, t := range tags {
		out = append(out, capability(project, q.Project, t, now))
	}
	return out, nil
}

func known[T any](v T, now time.Time) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "provider", Confidence: "known", ObservedAt: now}
}

// assigned is a fact the provider declares by policy (trust, economics), as the schema requires.
func assigned[T any](v T, now time.Time) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "assigned", Confidence: "known", ObservedAt: now}
}

func capability(project api.Project, projectName string, t gitlabtmpl.Tag, now time.Time) model.Capability {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	id := runnerIDPrefix + project.String() + ":" + t.Tag
	c.Identity.ID, c.Identity.Name = id, id
	c.Identity.Provider, c.Identity.Transport = ProviderID, "provider"
	c.Identity.Ownership = known("provider", now)
	c.Identity.Labels = []string{"arch=" + t.Arch, "os=" + t.OS, "nself.provider=" + ProviderID, LabelRepo + project.String(), LabelRunner + t.Tag}
	if projectName != "" {
		c.Identity.Labels = append(c.Identity.Labels, LabelProject+projectName)
	}
	c.Platform.OS, c.Platform.Arch = known(t.OS, now), known(t.Arch, now)
	c.Platform.Virtualization = known("vm", now)
	c.Resources.CPU, c.Resources.MemMB, c.Resources.DiskFreeMB = known(2.0, now), known(int64(8192), now), known(int64(25600), now)
	c.Availability.State, c.Availability.Slots = known("online", now), known(hostCapacity, now)
	c.Availability.Persistence = known(false, now)
	c.Location.Kind = known("hosted", now)
	c.Economics.Class = assigned("included", now)
	c.Trust.Accepts = assigned([]model.TrustClass{"owner", "internal", "collaborator", "untrusted"}, now)
	c.Trust.Isolation = assigned(model.Isolation("hosted-disposable"), now)
	c.Trust.Network = assigned([]model.NetworkScope{"internet"}, now)
	c.Trust.SecretClasses = assigned([]model.SecretClass{"none"}, now)
	c.Trust.PrivacyZone = assigned(model.PrivacyZone("hosted-allowed"), now)
	c.Separation.UIDSeparation, c.Separation.SigningEligible = assigned(true, now), assigned(false, now)
	c.Protocol.Versions, c.Protocol.Min = []int{1}, 1
	c.Verified = model.CapabilityVerified{By: "provider", At: now}
	c.Lifecycle.Discovered, c.Lifecycle.Eligible = true, true
	c.Lifecycle.AuthorizedProjects = []string{}
	if projectName != "" {
		c.Lifecycle.AuthorizedProjects = []string{projectName}
	}
	c.Identity.HostKeyFingerprints = []string{}
	c.Tools.Toolchains, c.Tools.Custom = map[string]model.Fact[string]{}, map[string]model.Fact[string]{}
	unknowns(reflect.ValueOf(&c).Elem(), now)
	return c
}

// unknowns marks every fact nobody set as unknown, so the capability validates.
func unknowns(v reflect.Value, now time.Time) {
	switch v.Kind() {
	case reflect.Struct:
		if strings.HasPrefix(v.Type().Name(), "Fact[") {
			if v.FieldByName("Source").String() == "" {
				v.FieldByName("Source").SetString("provider")
				v.FieldByName("Confidence").SetString("unknown")
				v.FieldByName("ObservedAt").Set(reflect.ValueOf(now))
			}
			return
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanSet() {
				unknowns(v.Field(i), now)
			}
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			elem := reflect.New(v.Type().Elem()).Elem()
			elem.Set(v.MapIndex(k))
			unknowns(elem, now)
			v.SetMapIndex(k, elem)
		}
	}
}
