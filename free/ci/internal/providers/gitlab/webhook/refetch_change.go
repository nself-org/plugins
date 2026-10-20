package webhook

// Purpose: the trust facts of a push, tag or merge request (Refetch, see refetch.go): one call per
// fact family so a failure in one never hides the others.
// Inputs:  r.f, the facts to refine.
// Outputs: r.f with api-sourced facts.
// Constraints: an API 404 can mean "no access" as well as "absent", so a missing object (not a member,
//          a commit outside the project) is only taken as a fact once the project itself has been read
//          with the same token.

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
)

// glProjectInfo is the part of a project the refetch reads.
type glProjectInfo struct {
	Path          string `json:"path_with_namespace"`
	Visibility    string `json:"visibility"`
	DefaultBranch string `json:"default_branch"`
}

func (r *fetch) change() {
	f := &r.f
	var proj glProjectInfo
	_, projOK := r.call(api.ProjectPath, r.params(), nil, &proj)
	if projOK {
		switch proj.Visibility {
		case "public", "internal", "private":
			f.Visibility = fact(proj.Visibility, "api", r.now)
		}
		// code that is pushed lives in the project itself
		if (f.Kind == "push" || f.Kind == "tag") && validName(proj.Path) {
			f.HeadRepo = fact(proj.Path, "api", r.now)
			f.Fork = fact(false, "api", r.now)
		}
	}
	switch f.Kind {
	case "pull_request":
		r.mergeRequest()
		r.protected(strings.TrimPrefix(f.BaseRef, "refs/heads/"), false)
	case "push":
		r.protected(strings.TrimPrefix(f.Ref, "refs/heads/"), true)
	case "tag":
		r.tagHead()
	}
	r.permissionAndBot(projOK)
	r.reachable(proj.DefaultBranch, projOK)
	r.changed()
}

// mergeRequest reads the merge request: the head it points at now and where it comes from.
func (r *fetch) mergeRequest() {
	var mr glMergeRequest
	if _, ok := r.call(mrPath, r.params("iid", strconv.Itoa(r.f.PRNumber)), nil, &mr); !ok {
		return
	}
	if mr.SHA != "" {
		r.f.HeadCurrent = fact(strings.EqualFold(mr.SHA, r.f.Revision), "api", r.now)
	}
	r.setSource(mr)
}

// protected reads the branch the run targets: whether it is protected and, for the pushed branch
// (head true), whether it still points at the revision.
func (r *fetch) protected(branch string, head bool) {
	if branch == "" {
		return
	}
	var b glRef
	if _, ok := r.call(api.BranchPath, r.params("branch", branch), nil, &b); !ok {
		return
	}
	r.f.ProtectedRef = fact(b.Protected, "api", r.now)
	if head && b.Commit.ID != "" {
		r.f.HeadCurrent = fact(strings.EqualFold(b.Commit.ID, r.f.Revision), "api", r.now)
	}
}

// tagHead re-resolves the tag: HeadCurrent is true only while it still points at the revision.
func (r *fetch) tagHead() {
	tag, isTag := strings.CutPrefix(r.f.Ref, "refs/tags/")
	if !isTag || tag == "" {
		return
	}
	var t glRef
	if _, ok := r.call(tagPath, r.params("tag", tag), nil, &t); ok && isHex40(strings.ToLower(t.Commit.ID)) {
		r.f.HeadCurrent = fact(strings.EqualFold(t.Commit.ID, r.f.Revision), "api", r.now)
	}
}

// accessLevel maps GitLab's member access level to the permission names the classifier knows.
func accessLevel(level int) string {
	switch {
	case level >= 50:
		return "admin"
	case level >= 40:
		return "maintain"
	case level >= 30:
		return "write" // Developer
	case level >= 20:
		return "triage" // Reporter
	case level > 0:
		return "read" // Guest, Minimal access
	}
	return "none"
}

// botName matches the user names GitLab gives project and group access token bots.
var botName = regexp.MustCompile(`^(project|group)_[0-9]+_bot`)

// permissionAndBot looks the actor up by name, then reads their access level on the project and
// whether the account is a bot.
func (r *fetch) permissionAndBot(projOK bool) {
	if r.f.Actor.Value == nil || *r.f.Actor.Value == "" {
		return
	}
	login := *r.f.Actor.Value
	var users []struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Bot      bool   `json:"bot"`
	}
	if _, ok := r.call(usersPath, nil, url.Values{"username": {login}}, &users); !ok {
		return
	}
	for _, u := range users {
		if !strings.EqualFold(u.Username, login) || u.ID <= 0 {
			continue
		}
		r.f.Bot = fact(u.Bot || botName.MatchString(u.Username), "api", r.now)
		var m struct {
			AccessLevel int `json:"access_level"`
		}
		status, ok := r.call(memberPath, r.params("user", itoa(u.ID)), nil, &m)
		switch {
		case ok:
			r.f.ActorPermission = fact(accessLevel(m.AccessLevel), "api", r.now)
		case status == http.StatusNotFound && projOK: // readable project, no membership
			r.f.ActorPermission = fact("none", "api", r.now)
		}
		return
	}
}

type comparison struct {
	Commits        []struct{} `json:"commits"`
	Diffs          []glDiff   `json:"diffs"`
	CompareTimeout bool       `json:"compare_timeout"`
}

type glDiff struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}

// reachable answers "the revision is reachable from the default branch": comparing the default branch
// to the revision lists the commits only the revision has, and none means it is an ancestor of (or the
// same as) the default branch. A 404 is "not in this project" once the project itself was read.
func (r *fetch) reachable(defaultBranch string, projOK bool) {
	if !isHex40(r.f.Revision) || defaultBranch == "" || !projOK {
		return
	}
	var cmp comparison
	status, ok := r.call(comparePath, r.params(), url.Values{"from": {defaultBranch}, "to": {r.f.Revision}}, &cmp)
	switch {
	case ok && !cmp.CompareTimeout:
		r.f.SHAReachable = fact(len(cmp.Commits) == 0, "api", r.now)
	case status == http.StatusNotFound:
		r.f.SHAReachable = fact(false, "api", r.now)
	}
}

// changed records the paths a change touches, both sides of a rename (a file moved out of a protected
// path must still show the path it left). A merge request lists its own diffs (paged); a push or tag
// compares the recorded base to the revision. More than 300 files is unknown.
func (r *fetch) changed() {
	if !isHex40(r.f.Revision) {
		return
	}
	var diffs []glDiff
	switch {
	case r.f.Kind == "pull_request":
		var ok bool
		if diffs, ok = r.mrDiffs(); !ok {
			return
		}
	case isHex40(r.f.BaseRevision):
		var cmp comparison
		q := url.Values{"from": {r.f.BaseRevision}, "to": {r.f.Revision}, "straight": {"true"}}
		if _, ok := r.call(comparePath, r.params(), q, &cmp); !ok || cmp.CompareTimeout {
			return
		}
		diffs = cmp.Diffs
	default:
		return
	}
	if len(diffs) > maxChangedFiles {
		return
	}
	names := make([]string, 0, len(diffs))
	for _, d := range diffs {
		names = append(names, d.NewPath)
		if d.OldPath != "" && d.OldPath != d.NewPath {
			names = append(names, d.OldPath)
		}
	}
	r.f.ChangedFiles = model.Fact[[]string]{Value: &names, Source: "api", ObservedAt: r.now, Confidence: "known"}
}

// mrDiffs lists the merge request's changed files, a page of 100 at a time. A fourth page is read so a
// merge request with exactly 300 files is told from one with more; more than 300 is not read on.
func (r *fetch) mrDiffs() ([]glDiff, bool) {
	var all []glDiff
	for page := 1; page <= maxChangedFiles/diffPage+1; page++ {
		q := url.Values{"per_page": {strconv.Itoa(diffPage)}, "page": {strconv.Itoa(page)}}
		var items []glDiff
		if _, ok := r.call(mrDiffsPath, r.params("iid", strconv.Itoa(r.f.PRNumber)), q, &items); !ok {
			return nil, false
		}
		all = append(all, items...)
		if len(items) < diffPage {
			return all, true
		}
	}
	return nil, false // still full after 400 files: far over the cap
}
