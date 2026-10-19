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
	"encoding/json"
	"fmt"
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
