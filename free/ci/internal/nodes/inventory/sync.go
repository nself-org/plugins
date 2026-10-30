package inventory

// Purpose: apply an inventory to the node registry (P7-AUTO-04): new hosts become discovered candidates;
//          for a node already enrolled sync sets only operator-owned attributes: assigned labels (pools
//          and rank included) through the NODE-30 label edit, projects removed by skip_repos, and, for
//          public_only, projects whose repository is not known to be public.
// Inputs:  Env (store, registry, actor, visibility reader), the entries, Options.
// Outputs: Summary: accepted, changes (each with its field source), candidates, rejected and notes.
// Constraints: never changes trust, host keys, isolation or secrets eligibility; never adds a project
//          authorization; a rejected entry never aborts the others; idempotent (a second identical run
//          changes nothing and writes no audit row); refused on a TEAM coordinator; a candidate that is
//          still waiting for `ci nodes add` (discovered, marked `inventory`, no project) is reported, not
//          edited; with Options.DryRun nothing is written.

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/nodes/labels"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Visibility answers whether a project's repository is public. known is false when it cannot be told.
type Visibility func(ctx context.Context, project string) (public, known bool)

// Env is what Sync needs.
type Env struct {
	Store      *store.Store
	Registry   *registry.Registry
	Actor      string
	Team       bool
	Visibility Visibility
	Now        func() time.Time
}

// Options of one run.
type Options struct{ DryRun bool }

// Change is one applied (or, with dry-run, planned) edit; Source names where the value came from.
type Change struct {
	Node   string `json:"node"`
	Field  string `json:"field"` // label | project
	Name   string `json:"name"`  // the label key or the project
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Reason string `json:"reason,omitempty"`
	Source string `json:"source"`
}

// Candidate is a host the registry did not know. Created is false for dry-run and for a pending one.
type Candidate struct {
	ID      string `json:"id"`
	Created bool   `json:"created"`
}

// Reject is an entry sync did not apply.
type Reject struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// Summary is the JSON result of one run.
type Summary struct {
	Schema     string      `json:"schema"`
	DryRun     bool        `json:"dry_run"`
	Accepted   []string    `json:"accepted"`
	Changed    []Change    `json:"changed"`
	Candidates []Candidate `json:"candidates"`
	Rejected   []Reject    `json:"rejected"`
	Notes      []string    `json:"notes"`
}

// Sync applies entries and returns the summary. A non-nil error is a refusal of the whole run.
func Sync(ctx context.Context, env Env, entries []Entry, opt Options) (Summary, error) {
	sum := Summary{Schema: SchemaID, DryRun: opt.DryRun, Accepted: []string{}, Changed: []Change{}, Candidates: []Candidate{}, Rejected: []Reject{}, Notes: []string{}}
	if env.Team {
		return sum, newErr(ReasonTeam, "sync is refused on a TEAM coordinator: its admin owns the nodes")
	}
	if env.Now == nil {
		env.Now = time.Now
	}
	s := &syncer{env: env, opt: opt, sum: &sum, seen: map[string]bool{}, vis: map[string][2]bool{},
		le: labels.Env{Store: env.Store, Registry: env.Registry, Actor: env.Actor}}
	for _, e := range entries {
		switch {
		case s.seen[e.ID] && e.ID != "":
			s.reject(e.ID, "id is given twice")
		default:
			s.seen[e.ID] = true
			if err := e.Check(); err != nil {
				s.reject(e.ID, err.Error())
				continue
			}
			s.one(ctx, e)
		}
	}
	return sum, nil
}

type syncer struct {
	env  Env
	opt  Options
	sum  *Summary
	seen map[string]bool
	vis  map[string][2]bool // project -> {public, known}, read once per run
	le   labels.Env
}

func (s *syncer) reject(id, why string) {
	s.sum.Rejected = append(s.sum.Rejected, Reject{ID: id, Reason: why})
}

func (s *syncer) one(ctx context.Context, e Entry) {
	node := e.ID
	var have registry.Node
	if e.Local {
		node = labels.LocalNode
	} else {
		n, err := s.env.Registry.Get(ctx, e.ID)
		var se *store.Error
		if errors.As(err, &se) && se.Code == "E650" {
			s.candidate(ctx, e)
			return
		}
		if err != nil {
			s.reject(e.ID, "the registry cannot be read")
			return
		}
		have = n
		if pending(n) {
			s.sum.Candidates = append(s.sum.Candidates, Candidate{ID: e.ID})
			return
		}
	}
	s.sum.Accepted = append(s.sum.Accepted, e.ID)
	if err := s.applyLabels(ctx, node, e); err != nil {
		s.reject(e.ID, "labels: "+oneLine(err))
		return
	}
	if !e.Local {
		s.applyProjects(ctx, have, e)
	}
}

// pending reports a candidate sync created that `ci nodes add` has not enrolled yet.
func pending(n registry.Node) bool {
	l := n.Capability.Lifecycle
	return l.Discovered && len(l.AuthorizedProjects) == 0 && slices.Contains(n.Capability.Identity.Labels, SourceLabel)
}

func (s *syncer) candidate(ctx context.Context, e Entry) {
	if e.SSH == "" {
		s.reject(e.ID, "a new host needs an ssh target")
		return
	}
	created := false
	if !s.opt.DryRun {
		if _, err := s.env.Registry.Register(ctx, NewCandidate(e, s.env.Now().UTC()), s.env.Actor); err != nil {
			s.reject(e.ID, "candidate: "+oneLine(err))
			return
		}
		created = true
	}
	s.sum.Candidates = append(s.sum.Candidates, Candidate{ID: e.ID, Created: created})
}

func (s *syncer) applyLabels(ctx context.Context, node string, e Entry) error {
	want, err := e.Want()
	if err != nil || len(want) == 0 {
		return err
	}
	have, err := s.le.Labels(ctx, node)
	if err != nil {
		return err
	}
	cur := map[string]string{}
	for _, l := range have {
		cur[l.Key] = l.Value
	}
	var diff []Change
	for _, l := range want {
		if v, ok := cur[l.Key]; !ok || v != l.Value {
			diff = append(diff, Change{Node: e.ID, Field: "label", Name: l.Key, From: cur[l.Key], To: l.Value, Source: SourceLabel})
		}
	}
	if len(diff) == 0 {
		return nil
	}
	if !s.opt.DryRun {
		if _, err = s.le.Edit(ctx, node, labels.Strings(want), nil); err != nil {
			return err
		}
	}
	s.sum.Changed = append(s.sum.Changed, diff...)
	return nil
}

func (s *syncer) applyProjects(ctx context.Context, n registry.Node, e Entry) {
	if len(e.SkipRepos) == 0 && !e.PublicOnly {
		return
	}
	for _, p := range slices.Clone(n.Capability.Lifecycle.AuthorizedProjects) {
		why := ""
		switch {
		case slices.Contains(e.SkipRepos, p):
			why = "skip_repos"
		case e.PublicOnly:
			why = s.notPublic(ctx, p)
		}
		if why == "" {
			continue
		}
		if !s.opt.DryRun {
			if _, err := s.env.Registry.Deauthorize(ctx, e.ID, p, s.env.Actor); err != nil {
				s.reject(e.ID, "project: "+oneLine(err))
				return
			}
		}
		s.sum.Changed = append(s.sum.Changed, Change{Node: e.ID, Field: "project", Name: p, From: "authorized", To: "removed", Reason: why, Source: SourceLabel})
	}
}

// notPublic returns why a project may not stay on a public_only node ("" when it may). Visibility is read
// once per project per run; an unreadable answer authorizes nothing and is reported once.
func (s *syncer) notPublic(ctx context.Context, p string) string {
	v, ok := s.vis[p]
	if !ok {
		if s.env.Visibility != nil {
			v[0], v[1] = s.env.Visibility(ctx, p)
		}
		s.vis[p] = v
		if !v[1] {
			s.sum.Notes = append(s.sum.Notes, "visibility unknown for project "+strconv.Quote(p)+": not authorized on public_only nodes")
		}
	}
	switch {
	case !v[1]:
		return "public_only: visibility unknown"
	case !v[0]:
		return "public_only: private repository"
	}
	return ""
}

func oneLine(err error) string {
	s := err.Error()
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
