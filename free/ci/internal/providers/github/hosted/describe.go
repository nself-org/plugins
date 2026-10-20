package hosted

// Purpose: Describe (Epic D5): the hosted runners GitHub offers for one repository, one per standard
//          label in templates/github/labels.yaml.
// Inputs:  providers.Query{Project, Repo "owner/name"}; the repository's visibility, read through
//          the REST funnel on every call (never taken from the query).
// Outputs: model.Capability values: identity.provider github-hosted, isolation hosted-disposable,
//          secret_classes [none], network internet, economics class included, privacy zone
//          hosted-allowed, and label nself.visibility=public for a public repository only.
// Constraints: a repository whose visibility cannot be read is described as private (the scheduler
//          then asks for an owner-declared ceiling, allowance.unknown); only labels from labels.yaml
//          appear (never a larger runner); an empty query describes nothing; the lookup is the only
//          request; a credential that does not resolve is E700 and reaches the caller.

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	githubtmpl "github.com/nself-org/plugins/free/ci/templates/github"
)

// Label keys of a hosted runner; Facts reads the same ones back from the leased node.
const (
	LabelRepo        = "nself.repo="
	LabelProject     = "nself.project="
	LabelRunner      = "nself.runner_label="
	LabelVisibility  = "nself.visibility="
	LabelPublic      = "nself.visibility=public"
	runnerIDPrefix   = "github-hosted:"
	visibilityPublic = "public"
)

// Describe implements providers.Provider.
func (p *Provider) Describe(ctx context.Context, q providers.Query) ([]model.Capability, error) {
	if !compat.V15() { // compat.V15(P7-HOST-03): no hosted capacity -> GitHub-hosted runners
		return nil, nil
	}
	if q.Repo == "" {
		return nil, nil
	}
	repo, ok := parseRepo(q.Repo)
	if !ok {
		return nil, misconfig("repo")
	}
	labels, err := githubtmpl.Labels()
	if err != nil {
		return nil, misconfig("labels")
	}
	c, err := p.client(ctx)
	if err != nil {
		return nil, err
	}
	visibility := "private"
	var info repoInfo
	// While the provider is down the lookup is not sent: every world build would otherwise be one more
	// request against a failing API. The runner is described as private (the rule for an unreadable
	// repository) and stays offline; the next probe or recovery describes it fully (P7-HOST-11).
	if providers.HealthOf(ctx, p.ID()).State != "down" {
		if _, err = c.Do(ctx, "GET", repoPath, repo.params(), nil, &info); err != nil {
			if isCredentialMissing(err) {
				return nil, err
			}
		} else if info.Visibility == visibilityPublic && !info.Private {
			visibility = visibilityPublic
		}
	}
	now := p.cfg.Clock().UTC().Truncate(time.Second)
	out := make([]model.Capability, 0, len(labels))
	for _, l := range labels {
		out = append(out, capability(repo, q.Project, visibility, l, now))
	}
	return out, nil
}

func isCredentialMissing(err error) bool {
	var pe *providers.Error
	return errors.As(err, &pe) && pe.Code == "E700"
}

func known[T any](v T, now time.Time) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "provider", Confidence: "known", ObservedAt: now}
}

// assigned is a fact the provider declares by policy (trust, economics), as the schema requires.
func assigned[T any](v T, now time.Time) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "assigned", Confidence: "known", ObservedAt: now}
}

func capability(repo repoRef, project, visibility string, l githubtmpl.Label, now time.Time) model.Capability {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	id := runnerIDPrefix + repo.String() + ":" + l.Label
	c.Identity.ID, c.Identity.Name = id, id
	c.Identity.Provider, c.Identity.Transport = ProviderID, "provider"
	c.Identity.Ownership = known("provider", now)
	c.Identity.Labels = []string{"arch=" + l.Arch, "os=" + l.OS, "nself.provider=" + ProviderID, LabelRepo + repo.String(), LabelRunner + l.Label, LabelVisibility + visibility}
	if project != "" {
		c.Identity.Labels = append(c.Identity.Labels, LabelProject+project)
	}
	c.Platform.OS, c.Platform.Arch = known(l.OS, now), known(l.Arch, now)
	c.Platform.Virtualization = known("vm", now)
	cpu, mem := 2.0, int64(7168)
	if visibility == visibilityPublic {
		cpu, mem = 4, 16384
	}
	if l.OS == "darwin" {
		cpu, mem = 3, 7168
	}
	c.Resources.CPU, c.Resources.MemMB, c.Resources.DiskFreeMB = known(cpu, now), known(mem, now), known(int64(14336), now)
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
	// Discovered means "found but not enrolled": the scheduler refuses such a runner (node.discovered),
	// so a hosted runner, authorized through the job's trust grants, must not carry it (P7-HOST-11).
	c.Lifecycle.Discovered, c.Lifecycle.Eligible = false, true
	c.Lifecycle.AuthorizedProjects = []string{}
	if project != "" {
		c.Lifecycle.AuthorizedProjects = []string{project}
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
