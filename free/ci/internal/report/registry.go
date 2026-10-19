package report

// Purpose: the reporter registry (P7-HOST-02, Epic D7). A status goes to the
// reporter whose remote kind matches the repository's remote: GitHub remotes
// to the gh adapter (the default), other kinds to the reporter their provider
// registers (GitLab: P7-HOST-06). There is one Reporter interface; this file
// only routes.
// Inputs:  RegisterReporter(kind, match, reporter) from init() of a provider.
// Outputs: Resolve(remote) -> the reporter, and its kind.
// Constraints: registered kinds are asked first, in registration order, then
// the github default; a kind registers once, except "github", which replaces
// the default.

import (
	"errors"
	"sync"
)

// KindGitHub is the kind of the default reporter.
const KindGitHub = "github"

type route struct {
	kind  string
	match func(remote string) bool
	rep   Reporter
}

// Registry maps a remote to the reporter that posts for it.
type Registry struct {
	mu     sync.RWMutex
	routes []route
	gh     route
}

// NewRegistry returns a registry whose github default is gh (nil: the gh
// adapter on the exec funnel).
func NewRegistry(gh Reporter) *Registry {
	if gh == nil {
		gh = GH{}
	}
	return &Registry{gh: route{KindGitHub, isGitHubRemote, gh}}
}

func isGitHubRemote(remote string) bool {
	_, _, ok := ParseRemote(remote)
	return ok
}

// Register adds a reporter for the remotes match accepts.
func (r *Registry) Register(kind string, match func(remote string) bool, rep Reporter) error {
	if kind == "" || match == nil || rep == nil {
		return errors.New("report: a reporter needs a kind, a matcher and a reporter")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if kind == KindGitHub {
		r.gh = route{kind, match, rep}
		return nil
	}
	for _, x := range r.routes {
		if x.kind == kind {
			return errors.New("report: reporter kind already registered: " + kind)
		}
	}
	r.routes = append(r.routes, route{kind, match, rep})
	return nil
}

// Resolve returns the reporter for remote, or false when none accepts it.
func (r *Registry) Resolve(remote string) (kind string, rep Reporter, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, x := range append(append([]route(nil), r.routes...), r.gh) {
		if matches(x.match, remote) {
			return x.kind, x.rep, true
		}
	}
	return "", nil, false
}

// matches asks a matcher; one that panics does not match. Matchers must be pure:
// they run under the registry's read lock and must not call Register.
func matches(match func(string) bool, remote string) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return match(remote)
}

// DefaultRegistry is the process-wide registry.
var DefaultRegistry = NewRegistry(nil)

// RegisterReporter adds a reporter to the process-wide registry.
func RegisterReporter(kind string, match func(remote string) bool, r Reporter) error {
	return DefaultRegistry.Register(kind, match, r)
}

// Resolve returns the process-wide reporter for remote.
func Resolve(remote string) (Reporter, bool) {
	_, rep, ok := DefaultRegistry.Resolve(remote)
	return rep, ok
}
