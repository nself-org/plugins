// Package claims decides which station runs commit CI for a sha (ADR 0032
// decision 5, P7-AUTO-10).
//
// Purpose: the status history of a commit is the ledger. Decide is a pure
// function over that history, so a lost state file never reruns a commit and
// a forged or foreign status never suppresses CI.
// Inputs:  Inputs (provider time, trusted creators, ledger, identity, limits).
// Outputs: a Decision: take, wait(until), skip or lost.
// Constraints: no I/O and no time.Now; ordering uses the provider's
// created_at; with no provider time yet no claim is declared dead (fail safe).
package claims

import (
	"sort"
	"strings"
	"time"
)

// DeadGrace is added to the run limit before a silent claim is dead.
const DeadGrace = 10 * time.Minute

// Kind is the outcome of Decide.
type Kind string

const (
	Take Kind = "take"
	Wait Kind = "wait"
	Skip Kind = "skip"
	Lost Kind = "lost"
)

// Status is one ledger entry.
type Status struct {
	Context     string
	State       string // pending, success, failure, error
	Creator     string
	Description string
	CreatedAt   time.Time
}

// Me is this station's identity.
type Me struct{ Host, Incarnation string }

// Inputs is everything Decide reads.
type Inputs struct {
	Now      time.Time // provider time (Date of the latest response); zero: unknown
	Statuses []Status
	Me       Me
	Trusted  []string // creators whose statuses count: the station login plus claims.trusted_creators
	RunLimit time.Duration
	// Preferred is the host that takes at once; empty means the lexically first of Hosts.
	Preferred string
	Hosts     []string
	Step      time.Duration // how long a non-preferred host waits
	FirstSeen time.Time     // provider time this host first saw the sha without a final result
	RetryMax  int
	RetryGap  time.Duration
	// ResultContext and ClaimContext default to nself-ci and nself-ci/claim.
	ResultContext, ClaimContext string
}

// Decision is the outcome. Holds is true when Me holds the earliest live
// claim (the publication fence reads it).
type Decision struct {
	Kind   Kind
	Until  time.Time
	Winner string
	Holds  bool
}

func (in Inputs) results() string {
	if in.ResultContext == "" {
		return "nself-ci"
	}
	return in.ResultContext
}

func (in Inputs) claimCtx() string {
	if in.ClaimContext == "" {
		return "nself-ci/claim"
	}
	return in.ClaimContext
}

func (in Inputs) trusted(creator string) bool {
	for _, t := range in.Trusted {
		if t != "" && t == creator {
			return true
		}
	}
	return false
}

// Final reports whether a trusted final result exists: success or failure, or
// RetryMax+1 trusted errors.
func (in Inputs) Final() bool {
	errs := 0
	for _, s := range in.Statuses {
		if s.Context != in.results() || !in.trusted(s.Creator) {
			continue
		}
		switch s.State {
		case "success", "failure":
			return true
		case "error":
			errs++
		}
	}
	return errs >= in.RetryMax+1
}

// ParseClaim reads "claim <host> <incarnation>".
func ParseClaim(desc string) (host, inc string, ok bool) {
	f := strings.Fields(desc)
	if len(f) != 3 || f[0] != "claim" {
		return "", "", false
	}
	return f[1], f[2], true
}

// hostOf returns the host a trusted status speaks for: a claim names it, a
// run status carries "host=<id>" in its description.
func hostOf(s Status) string {
	if h, _, ok := ParseClaim(s.Description); ok {
		return h
	}
	for _, f := range strings.Fields(s.Description) {
		if h, ok := strings.CutPrefix(f, "host="); ok {
			return h
		}
	}
	return ""
}

type claim struct {
	host string
	at   time.Time
}

// live returns the live claims, earliest first (ties by lexical host id).
func (in Inputs) live() []claim {
	cutoff := in.lastError() // a claim at or before a trusted error belongs to a finished attempt

	last := map[string]time.Time{} // newest trusted status per host
	first := map[string]time.Time{}
	for _, s := range in.Statuses {
		if !in.trusted(s.Creator) || !s.CreatedAt.After(cutoff) {
			continue
		}
		h := hostOf(s)
		if h == "" {
			continue
		}
		if s.CreatedAt.After(last[h]) {
			last[h] = s.CreatedAt
		}
		if s.Context == in.claimCtx() && s.State == "pending" {
			if _, _, ok := ParseClaim(s.Description); ok {
				if t, seen := first[h]; !seen || s.CreatedAt.Before(t) {
					first[h] = s.CreatedAt
				}
			}
		}
	}
	var out []claim
	for h, at := range first {
		if !in.Now.IsZero() && in.Now.Sub(last[h]) > in.RunLimit+DeadGrace {
			continue // dead on the provider clock
		}
		out = append(out, claim{h, at})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].at.Equal(out[j].at) {
			return out[i].at.Before(out[j].at)
		}
		return out[i].host < out[j].host
	})
	return out
}

func (in Inputs) lastError() time.Time {
	var t time.Time
	for _, s := range in.Statuses {
		if s.Context == in.results() && s.State == "error" && in.trusted(s.Creator) && s.CreatedAt.After(t) {
			t = s.CreatedAt
		}
	}
	return t
}

func (in Inputs) preferred() string {
	if in.Preferred != "" {
		return in.Preferred
	}
	hs := append([]string{}, in.Hosts...)
	sort.Strings(hs)
	if len(hs) == 0 {
		return in.Me.Host
	}
	return hs[0]
}

// Decide is the pure decision.
func Decide(in Inputs) Decision {
	if in.Final() {
		return Decision{Kind: Skip}
	}
	if l := in.live(); len(l) > 0 {
		if l[0].host == in.Me.Host {
			return Decision{Kind: Take, Winner: l[0].host, Holds: true}
		}
		return Decision{Kind: Lost, Winner: l[0].host}
	}
	// A retry candidate waits out the retry gap after the last trusted error.
	lastErr := in.lastError()
	if !lastErr.IsZero() && !in.Now.IsZero() && in.Now.Before(lastErr.Add(in.RetryGap)) {
		return Decision{Kind: Wait, Until: lastErr.Add(in.RetryGap)}
	}
	if in.Me.Host == in.preferred() {
		return Decision{Kind: Take}
	}
	if in.FirstSeen.IsZero() || in.Now.IsZero() {
		return Decision{Kind: Wait, Until: in.Now.Add(in.Step)}
	}
	if until := in.FirstSeen.Add(in.Step); in.Now.Before(until) {
		return Decision{Kind: Wait, Until: until}
	}
	return Decision{Kind: Take}
}

// MayPublish is the publication fence: re-read the ledger, then post the
// final result only if no final exists and Me still holds the earliest live
// claim. Otherwise the result stays local evidence.
func MayPublish(in Inputs) bool {
	d := Decide(in)
	return d.Kind == Take && d.Holds
}
