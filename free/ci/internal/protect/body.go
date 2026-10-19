package protect

// Purpose: turn the answer of GET /repos/{o}/{r}/branches/{b}/protection into
// the PUT body that adds the required `nself-ci` check and changes nothing
// else (P7-CI-07, X18). The same normaliser reads the answer after an apply,
// so "the PUT landed" is a comparison of two bodies, not of two guesses.
// Inputs:  the raw GET document (nil for an unprotected branch), the context
// name and the app id to pin it to.
// Outputs: a Plan (the PUT body, the checks it adds) or an error.
// Constraints: status checks are always sent as `checks` with an explicit app
// id (-1 = any app), never as `contexts` (GitHub refuses both together, and a
// contexts-only body would drop every app pin). A field the PUT API cannot
// express (an unknown key, required_signatures on) refuses the plan rather
// than silently dropping it. No network, no files.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Context is the required check name and AppID the GitHub Actions app that
// posts it (Epic D18: the pin stops a status from any other source satisfying it).
const (
	Context = "nself-ci"
	AppID   = 15368
)

// Check is one required status check with its app pin.
type Check struct {
	Context string `json:"context"`
	AppID   int    `json:"app_id"`
}

// Principals names who may push, dismiss or bypass; each list is sorted.
type Principals struct {
	Users []string `json:"users"`
	Teams []string `json:"teams"`
	Apps  []string `json:"apps"`
}

// StatusChecks is the PUT shape of the required_status_checks block.
type StatusChecks struct {
	Strict bool    `json:"strict"`
	Checks []Check `json:"checks"`
}

// Reviews is the PUT shape of the required_pull_request_reviews block.
type Reviews struct {
	DismissStale     bool        `json:"dismiss_stale_reviews"`
	RequireCodeOwner bool        `json:"require_code_owner_reviews"`
	ApprovingCount   int         `json:"required_approving_review_count"`
	RequireLastPush  bool        `json:"require_last_push_approval"`
	Dismissal        *Principals `json:"dismissal_restrictions,omitempty"`
	Bypass           *Principals `json:"bypass_pull_request_allowances,omitempty"`
}

// Body is the whole PUT document. Pointer fields marshal as null when nil,
// which is what the API wants for "no such rule".
type Body struct {
	RequiredStatusChecks    *StatusChecks `json:"required_status_checks"`
	EnforceAdmins           bool          `json:"enforce_admins"`
	RequiredReviews         *Reviews      `json:"required_pull_request_reviews"`
	Restrictions            *Principals   `json:"restrictions"`
	RequiredLinearHistory   bool          `json:"required_linear_history"`
	AllowForcePushes        bool          `json:"allow_force_pushes"`
	AllowDeletions          bool          `json:"allow_deletions"`
	BlockCreations          bool          `json:"block_creations"`
	RequiredConversationRes bool          `json:"required_conversation_resolution"`
	LockBranch              bool          `json:"lock_branch"`
	AllowForkSyncing        bool          `json:"allow_fork_syncing"`
}

// Plan is what apply would do.
type Plan struct {
	Repo, Branch string
	Unprotected  bool    // the branch had no protection: the body creates it
	Body         Body    // the PUT body
	Added        []Check // checks the body adds to the live ones (empty: nothing to do)
}

// Noop reports whether the live protection already requires the check.
func (p Plan) Noop() bool { return len(p.Added) == 0 && !p.Unprotected }

// JSON is the exact PUT body as indented JSON, newline terminated.
func (p Plan) JSON() []byte { return mustIndent(p.Body) }

func mustIndent(v any) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic("protect: marshal: " + err.Error()) // plain structs of strings, ints and bools cannot fail
	}
	return append(b, '\n')
}

type flag struct {
	Enabled bool `json:"enabled"`
}

type rawPrincipals struct {
	Users []struct{ Login string } `json:"users"`
	Teams []struct{ Slug string }  `json:"teams"`
	Apps  []struct{ Slug string }  `json:"apps"`
}

func (r *rawPrincipals) principals() *Principals {
	if r == nil {
		return nil
	}
	p := &Principals{Users: []string{}, Teams: []string{}, Apps: []string{}}
	for _, u := range r.Users {
		p.Users = append(p.Users, u.Login)
	}
	for _, t := range r.Teams {
		p.Teams = append(p.Teams, t.Slug)
	}
	for _, a := range r.Apps {
		p.Apps = append(p.Apps, a.Slug)
	}
	sort.Strings(p.Users)
	sort.Strings(p.Teams)
	sort.Strings(p.Apps)
	return p
}

type rawReviews struct {
	DismissStale     bool           `json:"dismiss_stale_reviews"`
	RequireCodeOwner bool           `json:"require_code_owner_reviews"`
	ApprovingCount   int            `json:"required_approving_review_count"`
	RequireLastPush  bool           `json:"require_last_push_approval"`
	Dismissal        *rawPrincipals `json:"dismissal_restrictions"`
	Bypass           *rawPrincipals `json:"bypass_pull_request_allowances"`
}

type rawStatusChecks struct {
	Strict   bool     `json:"strict"`
	Contexts []string `json:"contexts"`
	Checks   []struct {
		Context string `json:"context"`
		AppID   *int   `json:"app_id"`
	} `json:"checks"`
}

// knownKeys are the top-level keys of a GET answer this package can replay.
// url and required_signatures are read-only here (see Normalise).
var knownKeys = map[string]bool{
	"url": true, "required_status_checks": true, "enforce_admins": true, "required_pull_request_reviews": true,
	"restrictions": true, "required_linear_history": true, "allow_force_pushes": true, "allow_deletions": true,
	"block_creations": true, "required_conversation_resolution": true, "lock_branch": true,
	"allow_fork_syncing": true, "required_signatures": true,
}

// Normalise reads a GET answer into the canonical PUT body. It refuses a
// document it would have to drop fields from.
func Normalise(raw []byte) (Body, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return Body{}, fmt.Errorf("protect: the protection answer is not a JSON object: %w", err)
	}
	var unknown []string
	for k := range top {
		if !knownKeys[k] {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		return Body{}, fmt.Errorf("protect: the protection holds fields the PUT API cannot express: %s", strings.Join(unknown, ", "))
	}
	var sig flag
	if v, ok := top["required_signatures"]; ok {
		_ = json.Unmarshal(v, &sig)
	}
	if sig.Enabled {
		return Body{}, fmt.Errorf("protect: required_signatures is on and the protection PUT cannot replay it; change this repository by hand")
	}
	var b Body
	if err := decodeBlocks(top, &b); err != nil {
		return Body{}, err
	}
	return b, nil
}

func decodeBlocks(top map[string]json.RawMessage, b *Body) error {
	get := func(key string, into any) error {
		v, ok := top[key]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return nil
		}
		if err := json.Unmarshal(v, into); err != nil {
			return fmt.Errorf("protect: protection field %s: %w", key, err)
		}
		return nil
	}
	flags := map[string]*bool{
		"enforce_admins": &b.EnforceAdmins, "required_linear_history": &b.RequiredLinearHistory,
		"allow_force_pushes": &b.AllowForcePushes, "allow_deletions": &b.AllowDeletions, "block_creations": &b.BlockCreations,
		"required_conversation_resolution": &b.RequiredConversationRes, "lock_branch": &b.LockBranch,
		"allow_fork_syncing": &b.AllowForkSyncing,
	}
	for key, dst := range flags {
		var f flag
		if err := get(key, &f); err != nil {
			return err
		}
		*dst = f.Enabled
	}
	var rs *rawStatusChecks
	if err := get("required_status_checks", &rs); err != nil {
		return err
	}
	if rs != nil {
		b.RequiredStatusChecks = &StatusChecks{Strict: rs.Strict, Checks: foldChecks(rs)}
	}
	var rr *rawReviews
	if err := get("required_pull_request_reviews", &rr); err != nil {
		return err
	}
	if rr != nil {
		b.RequiredReviews = &Reviews{DismissStale: rr.DismissStale, RequireCodeOwner: rr.RequireCodeOwner,
			ApprovingCount: rr.ApprovingCount, RequireLastPush: rr.RequireLastPush,
			Dismissal: rr.Dismissal.principals(), Bypass: nonEmpty(rr.Bypass.principals())}
	}
	var rp *rawPrincipals
	if err := get("restrictions", &rp); err != nil {
		return err
	}
	b.Restrictions = rp.principals()
	return nil
}

// nonEmpty drops an all-empty bypass block: GitHub answers one for every
// protected branch and refuses to be sent an empty one back as meaningful.
func nonEmpty(p *Principals) *Principals {
	if p == nil || len(p.Users)+len(p.Teams)+len(p.Apps) == 0 {
		return nil
	}
	return p
}

// foldChecks lists the checks with their pins, then any context the answer
// names only in the legacy `contexts` list (those carry no pin: -1).
func foldChecks(rs *rawStatusChecks) []Check {
	out := []Check{}
	seen := map[string]bool{}
	for _, c := range rs.Checks {
		id := -1
		if c.AppID != nil {
			id = *c.AppID
		}
		out = append(out, Check{Context: c.Context, AppID: id})
		seen[c.Context] = true
	}
	for _, c := range rs.Contexts {
		if !seen[c] {
			out = append(out, Check{Context: c, AppID: -1})
		}
	}
	return out
}

// Build computes the plan from the live answer (nil = unprotected branch).
// The only difference between the live protection and Plan.Body is the added
// check. An already-required context under another app id is a conflict:
// changing an existing pin is not this tool's job.
func Build(repo, branch string, live []byte, appID int) (Plan, error) {
	want := Check{Context: Context, AppID: appID}
	p := Plan{Repo: repo, Branch: branch}
	if live == nil {
		p.Unprotected = true
		p.Body = Body{RequiredStatusChecks: &StatusChecks{Checks: []Check{want}}}
		p.Added = []Check{want}
		return p, nil
	}
	body, err := Normalise(live)
	if err != nil {
		return Plan{}, err
	}
	rs := body.RequiredStatusChecks
	if rs == nil {
		rs = &StatusChecks{Checks: []Check{}}
	}
	for _, c := range rs.Checks {
		if c.Context != want.Context {
			continue
		}
		if c.AppID != want.AppID {
			return Plan{}, fmt.Errorf("protect: %s is already required with app id %d, not %d; change a pin by hand", Context, c.AppID, want.AppID)
		}
		p.Body = body
		return p, nil
	}
	rs.Checks = append(rs.Checks, want)
	body.RequiredStatusChecks = rs
	p.Body, p.Added = body, []Check{want}
	return p, nil
}
