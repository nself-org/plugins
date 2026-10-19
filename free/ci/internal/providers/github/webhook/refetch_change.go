package webhook

// Purpose: the trust facts of a push, tag, pull request or dispatch (Refetch, see refetch.go): one
// call per fact family so a failure in one never hides the others.
// Inputs:  r.f, the facts to refine.
// Outputs: r.f with api-sourced facts.
// Constraints: an API 404 can mean "no access" as well as "absent", so a missing object is only taken
//          as a fact (SHA not reachable) once the repository itself has been read with the same token.

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

var permissions = map[string]bool{"admin": true, "maintain": true, "write": true, "triage": true, "read": true, "none": true}

func (r *fetch) change() {
	f := &r.f
	var repo ghRepo
	_, repoOK := r.call("/repos/{owner}/{repo}", r.params(), &repo)
	if repoOK {
		f.Visibility = fact(visibilityOf(repo), "api", r.now)
		if f.Kind != "pull_request" { // code that is pushed lives in the repository itself
			f.HeadRepo = fact(repo.FullName, "api", r.now)
			f.Fork = fact(false, "api", r.now)
		}
	}
	association := ""
	if f.Kind == "pull_request" {
		association = r.pullRequest()
	}
	r.firstTime(association)
	r.permissionAndBot()
	r.protected()
	r.compare(repo.DefaultBranch, repoOK)
}

// pullRequest reads the PR: the head it points at now, its repository and the author association.
func (r *fetch) pullRequest() string {
	var pr ghPR
	if _, ok := r.call("/repos/{owner}/{repo}/pulls/{number}", r.params("number", strconv.Itoa(r.f.PRNumber)), &pr); !ok {
		return ""
	}
	r.f.HeadCurrent = fact(strings.EqualFold(pr.Head.SHA, r.f.Revision), "api", r.now)
	if h := pr.Head.Repo; h != nil && h.FullName != "" {
		r.f.HeadRepo = fact(h.FullName, "api", r.now)
		base := int64(0)
		if pr.Base.Repo != nil {
			base = pr.Base.Repo.ID
		}
		r.f.Fork = fact(h.ID != base || h.Fork, "api", r.now)
	}
	if pr.AuthorAssociation != "" {
		r.f.AuthorAssociation = fact(pr.AuthorAssociation, "api", r.now)
	}
	return pr.AuthorAssociation
}

// firstTime maps GitHub's author_association (which counts prior commits to the repository).
func (r *fetch) firstTime(association string) {
	switch association {
	case "FIRST_TIME_CONTRIBUTOR", "FIRST_TIMER", "NONE":
		r.f.FirstTime = fact(true, "api", r.now)
	case "CONTRIBUTOR", "MEMBER", "OWNER", "COLLABORATOR":
		r.f.FirstTime = fact(false, "api", r.now)
	}
}

// permissionAndBot reads the sender's permission on the repository and whether the account is a bot.
func (r *fetch) permissionAndBot() {
	if r.f.Actor.Value == nil || *r.f.Actor.Value == "" {
		return
	}
	login := *r.f.Actor.Value
	var perm struct {
		Permission string `json:"permission"`
		RoleName   string `json:"role_name"`
	}
	if _, ok := r.call("/repos/{owner}/{repo}/collaborators/{user}/permission", r.params("user", login), &perm); ok {
		switch {
		case permissions[perm.RoleName]:
			r.f.ActorPermission = fact(perm.RoleName, "api", r.now)
		case permissions[perm.Permission]:
			r.f.ActorPermission = fact(perm.Permission, "api", r.now)
		}
	}
	var u ghUser
	if _, ok := r.call("/users/{user}", map[string]string{"user": login}, &u); ok && u.Type != "" {
		r.f.Bot = fact(u.Type == "Bot", "api", r.now)
	}
}

// protected reads branch protection of the branch the run targets: the PR base or the pushed branch.
func (r *fetch) protected() {
	ref := r.f.Ref
	if r.f.Kind == "pull_request" {
		ref = r.f.BaseRef
	}
	branch, isBranch := strings.CutPrefix(ref, "refs/heads/")
	seg, ok := refSegments(branch)
	if !isBranch || !ok {
		return
	}
	var b struct {
		Protected bool `json:"protected"`
	}
	if _, ok = r.call("/repos/{owner}/{repo}/branches/"+seg, r.params(), &b); ok {
		r.f.ProtectedRef = fact(b.Protected, "api", r.now)
	}
}

// compare answers SHA reachability and the changed files with one call. base is the recorded base
// revision, else the default branch (reachability only).
func (r *fetch) compare(defaultBranch string, repoOK bool) {
	if !isHex40(r.f.Revision) {
		return
	}
	base, withFiles := r.f.BaseRevision, true
	if base == "" {
		var ok bool
		if base, ok = refSegments(defaultBranch); !ok {
			return
		}
		withFiles = false
	}
	var cmp struct {
		Files []struct {
			Filename string `json:"filename"`
		} `json:"files"`
	}
	status, ok := r.call("/repos/{owner}/{repo}/compare/"+base+"..."+r.f.Revision, r.params(), &cmp)
	switch {
	case ok:
		r.f.SHAReachable = fact(true, "api", r.now)
	case status == http.StatusNotFound && repoOK:
		r.f.SHAReachable = fact(false, "api", r.now)
		return
	default:
		return
	}
	if withFiles && len(cmp.Files) < maxChangedFiles {
		names := make([]string, 0, len(cmp.Files))
		for _, file := range cmp.Files {
			names = append(names, file.Filename)
		}
		r.f.ChangedFiles = model.Fact[[]string]{Value: &names, Source: "api", ObservedAt: r.now, Confidence: "known"}
	}
}
