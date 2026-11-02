package gitbatch

// Purpose: the sessions of one run, one per repository root, reachable from the run's context so the
// call sites (internal/run, internal/evidence, internal/policy) need no new parameter.
// Inputs:  the run's context; a root.
// Outputs: *Session for the root, or nil when the run has no set (tests, other capture functions).
// Constraints: a set is only put on the context when the run's git goes through the default exec funnel
//          (a custom Capture must keep answering its own reads); Close ends every process.

import (
	"context"
	"path/filepath"
	"sync"
)

// Set owns the sessions of a run.
type Set struct {
	parent context.Context
	opt    Options
	mu     sync.Mutex
	by     map[string]*Session
	closed bool
}

// NewSet makes an empty set whose processes end with parent.
func NewSet(parent context.Context, opt Options) *Set {
	return &Set{parent: parent, opt: opt, by: map[string]*Session{}}
}

// For returns the session of root, creating it on first use. A closed set returns nil.
func (s *Set) For(root string) *Session {
	key := filepath.Clean(root)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	sess, ok := s.by[key]
	if !ok {
		sess = NewSession(s.parent, key, s.opt)
		s.by[key] = sess
	}
	return sess
}

// Close ends every session. Reads after it report no session.
func (s *Set) Close() {
	s.mu.Lock()
	s.closed = true
	all := make([]*Session, 0, len(s.by))
	for _, sess := range s.by {
		all = append(all, sess)
	}
	s.mu.Unlock()
	for _, sess := range all {
		sess.Close()
	}
}

type setKey struct{}

// WithSet returns ctx carrying set.
func WithSet(ctx context.Context, set *Set) context.Context {
	return context.WithValue(ctx, setKey{}, set)
}

// Has reports whether ctx already carries a set.
func Has(ctx context.Context) bool { s, _ := ctx.Value(setKey{}).(*Set); return s != nil }

// For is the session of root in ctx's set, or nil: the caller then runs git itself.
func For(ctx context.Context, root string) *Session {
	set, _ := ctx.Value(setKey{}).(*Set)
	if set == nil || root == "" {
		return nil
	}
	return set.For(root)
}
