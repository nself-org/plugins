package roles

// Purpose: Authorize and AuthorizeAction (Epic D19), the calls coordinator operations make before they
// act, over actors resolved from a user certificate, a scoped token, or the local operator.
// Inputs:  an Actor, a minimum role or an action name, and a project id.
// Outputs: nil, or an E687 *Error; one audit row per decision for release- and admin-level checks.
// Constraints: three roles only; an actor scoped to projects cannot do a project-less release or admin
//          action; the operator is the local host's own access (data-dir access is admin access, EPIC D1)
//          and is allowed and still audited; an unaudited decision (member-level reads) writes no row; a
//          failed audit write fails the decision closed.

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Kind says how an actor was identified.
type Kind string

// Actor kinds.
const (
	KindOperator Kind = "operator"
	KindUser     Kind = "user"
	KindToken    Kind = "token"
)

// AllProjects in a grant or token means every project.
const AllProjects = "*"

// Actor is a resolved identity. Only a Service builds a usable one (every field is unexported, so a
// caller cannot raise its own role); the zero Actor is refused. A decision re-reads the grant, token and
// certificate state, so a revocation takes effect on an Actor that was resolved earlier.
type Actor struct {
	id       string
	kind     Kind
	role     Role
	projects []string
	tokenID  string
	serial   string    // hex certificate serial of a user-certificate actor
	notAfter time.Time // end of that certificate's validity
	svc      *Service
}

// ID is the principal id (the operator's id carries a "cli:" prefix).
func (a Actor) ID() string { return a.id }

// Kind says how the actor was identified.
func (a Actor) Kind() Kind { return a.kind }

// Role is the role the actor held when it was last resolved.
func (a Actor) Role() Role { return a.role }

// Projects is a copy of the actor's project list ("*" for all).
func (a Actor) Projects() []string { return append([]string(nil), a.projects...) }

// Label is the actor as it appears in the audit log.
func (a Actor) Label() string {
	if a.kind == KindToken && a.tokenID != "" {
		return "token:" + a.tokenID + ":" + a.id
	}
	return string(a.kind) + ":" + a.id
}

func (a Actor) allProjects() bool {
	for _, p := range a.projects {
		if p == AllProjects {
			return true
		}
	}
	return false
}

func (a Actor) allows(project string) bool {
	if a.allProjects() {
		return true
	}
	for _, p := range a.projects {
		if project != "" && p == project {
			return true
		}
	}
	return false
}

// Service holds the store and the audit log the decisions use.
type Service struct {
	St   *store.Store
	Log  *audit.Log
	Now  func() time.Time
	Self string // the operator's own principal id; `--self` identities are issued only to it

	lim signInLimiter // the rate limit of the sign-in failure rows (signin.go)
}

// New returns a Service over st writing audit rows through log.
func New(st *store.Store, log *audit.Log) *Service { return &Service{St: st, Log: log, Now: time.Now} }

var current atomic.Pointer[Service]

// Use installs the process-wide Service that NonSelfPrincipals reads.
func Use(s *Service) { current.Store(s) }

// Operator is the local host's own actor (CLI access to the data dir). It holds every role on every project.
func (s *Service) Operator(id string) Actor {
	return Actor{id: id, kind: KindOperator, role: Admin, projects: []string{AllProjects}, svc: s}
}

type authKey struct{}

// WithAuthor records the author of the object being approved, for run.approve's non-author rule.
func WithAuthor(ctx context.Context, author string) context.Context {
	return context.WithValue(ctx, authKey{}, author)
}

type decision struct {
	action, target, project string
	min                     Role
	audited, needProject    bool
	nonAuthor               bool
	forced                  error
}

// Can is the pure check on the Actor as it was resolved: no store access, no revocation check and no
// audit row. Authorization decisions go through Authorize, AuthorizeAction or the Service methods, which
// re-read the actor first.
func (a Actor) Can(role Role, project string) error {
	if a.svc == nil || a.id == "" || a.kind == "" {
		return fail(ReasonNoActor, "no identity")
	}
	if role.Rank() == 0 {
		return fail(ReasonUnknownRole, "unknown role %q", string(role))
	}
	if a.role.Rank() < role.Rank() {
		return fail(ReasonRole, "%s needs role %s", a.Label(), role)
	}
	if a.kind != KindOperator && project == "" && role.Rank() > Member.Rank() && !a.allProjects() {
		return fail(ReasonProject, "%s is scoped to projects and cannot do a project-wide action", a.Label())
	}
	if project != "" && !a.allows(project) {
		return fail(ReasonProject, "%s has no access to project %s", a.Label(), project)
	}
	return nil
}

// Authorize requires the actor to hold at least role on project ("" for a coordinator-wide operation).
func Authorize(ctx context.Context, a Actor, role Role, project string) error {
	return a.decide(ctx, decision{action: "authorize", target: string(role), project: project, min: role, audited: role != Member})
}

// AuthorizeAction maps action to its minimum role and extra rules (project scope, non-author approval).
func AuthorizeAction(ctx context.Context, a Actor, action, project string) error {
	r, ok := catalogue[action]
	if !ok {
		return a.decide(ctx, decision{action: "authorize", target: action, project: project, min: Admin, audited: true, forced: fail(ReasonUnknownAct, "unknown action %q", action)})
	}
	return a.decide(ctx, decision{action: "authorize", target: action, project: project, min: r.min, audited: r.audited, needProject: r.scoped, nonAuthor: r.needsAuthr})
}
