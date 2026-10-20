package runners

// Purpose: Mint, the POST /user/runners call (Epic D11, review F6, F14).
// Inputs:  a providers.RunnerRequest (its Scope.Repo is required, "group/project"); MintOptions.
// Outputs: the runner authentication token as a providers.RunnerCredential (Kind gitlab-runner-token,
//          expiry now + CredentialLifetime). MintRunner also returns the runner id and description
//          Deregister needs.
// Constraints: a reply that does not decode (an id as a string) is E707 mint.rejected and the runner GitLab may
//          have created is not deleted here: it carries the nself-glr- description and the orphan sweep removes it
//          after 70 minutes.
//          runner_type is project_type, run_untagged is false and locked is true, always. The tag
//          list is the operator's providers.gitlab.runner.tags plus one unique nself-glr-<ulid> tag,
//          never copied from the request or from a demand (a demand with other tags is refused by the
//          demand handler as E708 label.not_allowed); the description is the same name. access_level is
//          not_protected unless MintOptions.ProtectedOnly, which only a verified Restriction sets. A reply
//          whose token is not a glrt- token (or has no runner id) is E707 and the runner it may have
//          created is deleted again at once. The token never reaches a log, an error or the store; Minted
//          prints without it.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// Access levels of a runner.
const (
	AccessNotProtected = "not_protected"
	AccessRefProtected = "ref_protected"
)

// MintOptions are the parts of a registration a caller may choose.
type MintOptions struct {
	// ProtectedOnly registers the runner as ref_protected. Set it only from a verified Restriction.
	ProtectedOnly bool
	// Tags, when set, are the configured tags the runner carries besides its unique one (a verified subset of
	// providers.gitlab.runner.tags, from Assess): GitLab hands a runner any pending job whose tags it holds,
	// so a runner that carries more tags than its job can take another job meant for other hardware. Unset:
	// every configured tag except the architecture tags that contradict Arch.
	Tags []string
	// Arch is amd64 or arm64; it only matters when Tags is unset.
	Arch string
}

// Minted is one registration: the credential plus what teardown needs.
type Minted struct {
	RunnerID    string
	Description string
	Tags        []string
	AccessLevel string
	ProjectID   int64
	Scope       providers.RunnerScope
	Credential  providers.RunnerCredential
}

// Ref is the handle to pass to Deregister.
func (m Minted) Ref() providers.RunnerRef {
	return providers.RunnerRef{ID: m.RunnerID, Scope: m.Scope}
}

// String, GoString, LogValue and MarshalJSON leave the credential out wherever a Minted is printed.
func (m Minted) String() string {
	return fmt.Sprintf("gitlab runner %s (id %s, %s)", m.Description, m.RunnerID, m.AccessLevel)
}
func (m Minted) GoString() string { return m.String() }
func (m Minted) LogValue() slog.Value {
	return slog.GroupValue(slog.String("description", m.Description), slog.String("id", m.RunnerID), slog.String("access", m.AccessLevel))
}
func (m Minted) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		RunnerID    string   `json:"runner_id"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		AccessLevel string   `json:"access_level"`
	}{m.RunnerID, m.Description, m.Tags, m.AccessLevel})
}

type mintBody struct {
	RunnerType  string   `json:"runner_type"`
	ProjectID   int64    `json:"project_id"`
	Description string   `json:"description"`
	TagList     []string `json:"tag_list"`
	RunUntagged bool     `json:"run_untagged"`
	Locked      bool     `json:"locked"`
	AccessLevel string   `json:"access_level"`
}

type mintReply struct {
	ID    int64  `json:"id"`
	Token string `json:"token"`
}

// MintRunner creates one single-job project runner and returns the credential with its teardown handle.
func (r *Registrar) MintRunner(ctx context.Context, req providers.RunnerRequest, opt MintOptions) (Minted, error) {
	p, err := project(req.Scope)
	if err != nil {
		return Minted{}, err
	}
	pid, err := r.projectID(ctx, p)
	if err != nil {
		return Minted{}, err
	}
	now := r.now()
	id, err := newULID(now, r.entropy)
	if err != nil {
		return Minted{}, e707("mint.entropy")
	}
	name := RunnerPrefix + id
	tags, err := r.tagsFor(opt)
	if err != nil {
		return Minted{}, err
	}
	tags = append(tags, name)
	access := AccessNotProtected
	if opt.ProtectedOnly {
		access = AccessRefProtected
	}
	body := mintBody{RunnerType: "project_type", ProjectID: pid, Description: name, TagList: tags, RunUntagged: false, Locked: true, AccessLevel: access}
	var out mintReply
	if _, err = r.c.Do(ctx, "POST", userRunnersPath, nil, body, &out); err != nil {
		return Minted{}, runnerErr(err, "mint")
	}
	if out.ID <= 0 {
		return Minted{}, e707("mint.incomplete")
	}
	if !strings.HasPrefix(out.Token, TokenPrefix) {
		// not an authentication token: the runner exists, so remove it before refusing
		if derr := r.remove(context.WithoutCancel(ctx), out.ID); derr != nil {
			return Minted{}, e707("mint.token_format_runner_left")
		}
		return Minted{}, e707("mint.token_format")
	}
	return Minted{
		RunnerID: itoa(out.ID), Description: name, Tags: tags, AccessLevel: access, ProjectID: pid, Scope: req.Scope,
		Credential: providers.RunnerCredential{Kind: CredentialKind, Value: out.Token, ExpiresAt: now.Add(CredentialLifetime)},
	}, nil
}

// tagsFor chooses the configured tags of a new runner.
func (r *Registrar) tagsFor(opt MintOptions) ([]string, error) {
	if len(opt.Tags) > 0 {
		configured := make(map[string]bool, len(r.set.Tags))
		for _, t := range r.set.Tags {
			configured[t] = true
		}
		for _, t := range opt.Tags {
			if !configured[t] {
				return nil, e702("runner.tags")
			}
		}
		return append([]string(nil), opt.Tags...), nil
	}
	var out []string
	for _, t := range r.set.Tags {
		if contradicts(t, opt.Arch) {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// contradicts reports an architecture tag that names the other architecture than arch.
func contradicts(tag, arch string) bool {
	switch strings.ToLower(tag) {
	case "arm64":
		return arch == "amd64"
	case "amd64", "x64":
		return arch == "arm64"
	}
	return false
}

// remove deletes a runner by number without the ownership check (the caller has just created it).
func (r *Registrar) remove(ctx context.Context, id int64) error {
	resp, err := r.c.Do(ctx, "DELETE", runnerPath, map[string]string{"runner": itoa(id)}, nil, nil)
	if err != nil && resp.Status != 404 {
		return err
	}
	return nil
}

// Mint implements providers.RunnerRegistrar. The runner is not_protected: the caller that verified a
// Restriction uses MintRunner with MintOptions.
func (r *Registrar) Mint(ctx context.Context, req providers.RunnerRequest) (providers.RunnerCredential, error) {
	m, err := r.MintRunner(ctx, req, MintOptions{})
	return m.Credential, err
}
