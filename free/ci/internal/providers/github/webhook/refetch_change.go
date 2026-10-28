package webhook

// Purpose: the trust facts of a push, tag, pull request or dispatch (Refetch, see refetch.go): one
// call per fact family so a failure in one never hides the others.
// Inputs:  r.f, the facts to refine.
// Outputs: r.f with api-sourced facts.
// Constraints: an API 404 can mean "no access" as well as "absent", so a missing object is only taken
//          as a fact (SHA not reachable) once the repository itself has been read with the same token.

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

var errUnresolvedTag = errors.New("tag could not be resolved")

var permissions = map[string]bool{"admin": true, "maintain": true, "write": true, "triage": true, "read": true, "none": true}

func (r *fetch) change() {
	f := &r.f
	var repo ghRepo
	_, repoOK := r.call("/repos/{owner}/{repo}", r.params(), &repo)
	if repoOK {
		if v, known := visibilityOf(repo); known {
			f.Visibility = fact(v, "api", r.now)
		}
		// code that is pushed lives in the repository itself; a dispatch revision is the caller's choice and
		// may be a commit of the fork network, so it says nothing about the head repository
		if (f.Kind == "push" || f.Kind == "tag") && repo.FullName != "" {
			f.HeadRepo = fact(repo.FullName, "api", r.now)
			f.Fork = fact(false, "api", r.now)
		}
	}
	association := ""
	if f.Kind == "pull_request" {
		association = r.pullRequest()
	} else if f.Kind == "push" {
		r.pushHead()
	} else if f.Kind == "tag" {
		r.tagHead()
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
	if pr.Head.SHA != "" {
		r.f.HeadCurrent = fact(strings.EqualFold(pr.Head.SHA, r.f.Revision), "api", r.now)
	}
	baseID, _ := strconv.ParseInt(r.f.RepoID, 10, 64) // the verified base repository id
	if pr.Base.Repo != nil && pr.Base.Repo.ID > 0 {
		baseID = pr.Base.Repo.ID
	}
	if h := pr.Head.Repo; h != nil && h.FullName != "" && h.ID > 0 && baseID > 0 {
		r.f.HeadRepo = fact(h.FullName, "api", r.now)
		// a fork is a PR whose head lives in another repository (h.Fork says the head repository is itself
		// a fork of something, which is true of a same-repository PR in a forked repository too)
		r.f.Fork = fact(h.ID != baseID, "api", r.now)
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

type comparison struct {
	Files []struct {
		Filename         string `json:"filename"`
		PreviousFilename string `json:"previous_filename"`
	} `json:"files"`
}

// compare answers SHA reachability and the changed files with two independent reads.
//
// Changed files come from base...revision (the recorded base revision; absent for a new branch, and
// possibly gone after a force-push, in which case the files stay unknown).
//
// Reachability means "the revision is reachable from the default branch": compare answers 200 for
// any commit of the repository network, so a 200 alone proves nothing. Only the status says where the
// revision sits: "behind" (the revision is an ancestor of the default branch) or "identical" is
// reachable; "ahead" or "diverged" is an unmerged commit (a fork head, an unprotected branch) and is
// not. A 404 on the default branch compare is "not in this repository" once the repository itself was
// read with the same token. The comparison starts from the sha of the default branch head, not its name.
func (r *fetch) compare(defaultBranch string, repoOK bool) {
	if !isHex40(r.f.Revision) {
		return
	}
	if base := r.f.BaseRevision; isHex40(base) {
		var cmp comparison
		if _, ok := r.call("/repos/{owner}/{repo}/compare/"+base+"..."+r.f.Revision, r.params(), &cmp); ok {
			r.setChangedFiles(cmp)
		}
	}
	seg, ok := refSegments(defaultBranch)
	if !ok || !repoOK {
		return
	}
	// the default branch is read to its head sha first and the compare starts from that sha, never from the
	// branch name: git resolves refs/tags/<name> before refs/heads/<name>, so a tag named like the default
	// branch would win the name and make the compare list nothing. The head read is one more call from the
	// same budget; its failure leaves the fact unknown.
	var head struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if _, ok = r.call("/repos/{owner}/{repo}/branches/"+seg, r.params(), &head); !ok || !isHex40(head.Commit.SHA) {
		return
	}
	var onDefault struct {
		Status string `json:"status"`
	}
	status, ok := r.call("/repos/{owner}/{repo}/compare/"+strings.ToLower(head.Commit.SHA)+"..."+r.f.Revision, r.params(), &onDefault)
	switch {
	case ok && (onDefault.Status == "behind" || onDefault.Status == "identical"):
		r.f.SHAReachable = fact(true, "api", r.now)
	case ok && (onDefault.Status == "ahead" || onDefault.Status == "diverged"):
		r.f.SHAReachable = fact(false, "api", r.now)
	case status == http.StatusNotFound:
		r.f.SHAReachable = fact(false, "api", r.now)
	}
}

// setChangedFiles records the paths a change touches, both sides of a rename (a file moved out of a
// protected path must still show the path it left). 300 files or more is unknown.
func (r *fetch) setChangedFiles(c comparison) {
	if len(c.Files) >= maxChangedFiles {
		return
	}
	names := make([]string, 0, len(c.Files))
	for _, file := range c.Files {
		names = append(names, file.Filename)
		if file.PreviousFilename != "" {
			names = append(names, file.PreviousFilename)
		}
	}
	r.f.ChangedFiles = model.Fact[[]string]{Value: &names, Source: "api", ObservedAt: r.now, Confidence: "known"}
}

// pushHead re-reads the pushed branch: HeadCurrent is true only while the branch still points at the
// revision, so a refetch at classification time sees a ref that moved after the delivery.
func (r *fetch) pushHead() {
	branch, isBranch := strings.CutPrefix(r.f.Ref, "refs/heads/")
	seg, ok := refSegments(branch)
	if !isBranch || !ok {
		return
	}
	var ref refObject
	if _, ok = r.call("/repos/{owner}/{repo}/git/ref/heads/"+seg, r.params(), &ref); ok && ref.Object.SHA != "" {
		r.f.HeadCurrent = fact(strings.EqualFold(ref.Object.SHA, r.f.Revision), "api", r.now)
	}
}

// tagHead re-resolves the tag: HeadCurrent is true only while it still points at the revision (an
// annotated tag is dereferenced to its commit, as at delivery).
func (r *fetch) tagHead() {
	tag, isTag := strings.CutPrefix(r.f.Ref, "refs/tags/")
	if !isTag || tag == "" {
		return
	}
	r.calls++
	if rev, ok := resolveTagWith(r.ctx, r.c, r.f.Repo, tag); ok {
		r.okCalls++
		r.f.HeadCurrent = fact(strings.EqualFold(rev, r.f.Revision), "api", r.now)
	} else if r.firstErr == nil {
		r.firstErr = errUnresolvedTag
	}
}
