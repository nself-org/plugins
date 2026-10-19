package protect

// Purpose: read the answer of GET /repos/{o}/{r}/branches/{b}/protection into
// the canonical PUT Body (P7-CI-07). Build uses it for the live protection and
// Apply uses it again on the read-back, so "the PUT landed" is a comparison of
// two bodies.
// Inputs:  the raw GET document.
// Outputs: a Body, or an error naming what cannot be replayed.
// Constraints: a key the normaliser does not know, at the top level or inside
// a block, refuses the plan: dropping it would reset it on the PUT, and the
// read-back goes through this same code, so the loss would never show. The
// answer's url fields are ignored. required_signatures on refuses the plan
// (the protection PUT cannot set it). No network, no files.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

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

func keys(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

// knownKeys are the keys of a GET answer this package can replay, per block
// ("" is the top level). url fields are read-only. required_signatures is
// read-only here (see Normalise).
var knownKeys = map[string]map[string]bool{
	"": keys("url", "required_status_checks", "enforce_admins", "required_pull_request_reviews", "restrictions",
		"required_linear_history", "allow_force_pushes", "allow_deletions", "block_creations",
		"required_conversation_resolution", "lock_branch", "allow_fork_syncing", "required_signatures"),
	"flag":                          keys("url", "enabled"),
	"required_status_checks":        keys("url", "strict", "contexts", "contexts_url", "checks", "enforcement_level"), // enforcement_level: deprecated, derived by GitHub, not settable
	"required_status_checks.check":  keys("context", "app_id"),
	"required_pull_request_reviews": keys("url", "dismiss_stale_reviews", "require_code_owner_reviews", "require_last_push_approval", "required_approving_review_count", "dismissal_restrictions", "bypass_pull_request_allowances"),
	"principals":                    keys("url", "users_url", "teams_url", "apps_url", "users", "teams", "apps"),
}

// unknownIn lists the keys of the JSON object raw that are not in allowed.
func unknownIn(raw json.RawMessage, allowed map[string]bool) []string {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return nil // a non-object is reported by the typed decode that follows
	}
	var out []string
	for k := range m {
		if !allowed[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// checkBlocks refuses a document with an unknown key in any block it replays.
func checkBlocks(top map[string]json.RawMessage) error {
	var bad []string
	add := func(where string, keys []string) {
		for _, k := range keys {
			bad = append(bad, strings.TrimPrefix(where+"."+k, "."))
		}
	}
	add("", unknownIn(mustMarshal(top), knownKeys[""]))
	for _, k := range []string{"enforce_admins", "required_linear_history", "allow_force_pushes", "allow_deletions", "block_creations",
		"required_conversation_resolution", "lock_branch", "allow_fork_syncing", "required_signatures"} {
		if v, ok := top[k]; ok {
			add(k, unknownIn(v, knownKeys["flag"]))
		}
	}
	if v, ok := top["required_status_checks"]; ok {
		add("required_status_checks", unknownIn(v, knownKeys["required_status_checks"]))
		var rs struct {
			Checks []json.RawMessage `json:"checks"`
		}
		_ = json.Unmarshal(v, &rs)
		for _, c := range rs.Checks {
			add("required_status_checks.checks[]", unknownIn(c, knownKeys["required_status_checks.check"]))
		}
	}
	if v, ok := top["required_pull_request_reviews"]; ok {
		add("required_pull_request_reviews", unknownIn(v, knownKeys["required_pull_request_reviews"]))
		var rr map[string]json.RawMessage
		_ = json.Unmarshal(v, &rr)
		for _, k := range []string{"dismissal_restrictions", "bypass_pull_request_allowances"} {
			if p, ok := rr[k]; ok {
				add("required_pull_request_reviews."+k, unknownIn(p, knownKeys["principals"]))
			}
		}
	}
	if v, ok := top["restrictions"]; ok {
		add("restrictions", unknownIn(v, knownKeys["principals"]))
	}
	if len(bad) > 0 {
		return fmt.Errorf("protect: the protection holds fields the PUT API cannot express: %s", strings.Join(bad, ", "))
	}
	return nil
}

func mustMarshal(m map[string]json.RawMessage) json.RawMessage {
	b, _ := json.Marshal(m)
	return b
}

// Normalise reads a GET answer into the canonical PUT body. It refuses a
// document it would have to drop fields from.
func Normalise(raw []byte) (Body, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return Body{}, fmt.Errorf("protect: the protection answer is not a JSON object: %w", err)
	}
	if err := checkBlocks(top); err != nil {
		return Body{}, err
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
