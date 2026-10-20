package mode

// Purpose: the world coordinator-facts provider (SCHED D22). While a serve runs, the world learns
// {Serving: true, Mode, AgentPeers}; with no serve it learns Serving false, so a plain `ci run` is never
// treated as a coordinator host.
// Inputs:  Begin (called by a serve after Start passed) and its Options.
// Outputs: Facts, registered once with world.RegisterCoordinatorFacts through RegisterProvider.
// Constraints: the mode is read from the store on every call, fail closed (unreadable = team); with two
//          serves in one process (tests) the stricter mode wins; Peers is optional and a missing peer map
//          makes the world treat agent nodes as the coordinator host (its own fail-closed rule). The
//          registry is process-wide and holds no secret.

import (
	"context"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// Options describes one running serve.
type Options struct {
	Store *store.Store
	Peers func() map[string]string // node id to last observed agent peer IP; nil = unknown
}

// Session is one running serve. End it when the serve stops.
type Session struct {
	opts Options
	once sync.Once
}

var active struct {
	sync.Mutex
	sessions map[*Session]struct{}
}

var registerOnce struct {
	sync.Mutex
	done bool
}

// Begin marks a serve as running; Facts reports Serving true until End is called.
func Begin(o Options) *Session {
	s := &Session{opts: o}
	active.Lock()
	defer active.Unlock()
	if active.sessions == nil {
		active.sessions = map[*Session]struct{}{}
	}
	active.sessions[s] = struct{}{}
	return s
}

// End stops reporting this serve. It is safe to call twice.
func (s *Session) End() {
	s.once.Do(func() {
		active.Lock()
		delete(active.sessions, s)
		active.Unlock()
	})
}

// Serving reports whether any serve is running in this process.
func Serving() bool {
	active.Lock()
	defer active.Unlock()
	return len(active.sessions) > 0
}

// Facts is the provider: Serving false when no serve runs.
func Facts(ctx context.Context) (world.CoordinatorFacts, error) {
	active.Lock()
	list := make([]*Session, 0, len(active.sessions))
	for s := range active.sessions {
		list = append(list, s)
	}
	active.Unlock()
	if len(list) == 0 {
		return world.CoordinatorFacts{}, nil
	}
	out := world.CoordinatorFacts{Serving: true, Mode: Personal}
	for _, s := range list {
		m := Team
		if s.opts.Store != nil {
			m = s.opts.Store.CoordinatorMode(ctx)
		}
		if m != Personal {
			out.Mode = Team
		}
		if s.opts.Peers != nil {
			if out.AgentPeers == nil {
				out.AgentPeers = map[string]string{}
			}
			for k, v := range s.opts.Peers() {
				out.AgentPeers[k] = v
			}
		}
	}
	return out, nil
}

// RegisterProvider registers Facts with the world, once per process; later calls are no-ops.
func RegisterProvider() error {
	registerOnce.Lock()
	defer registerOnce.Unlock()
	if registerOnce.done {
		return nil
	}
	if err := world.RegisterCoordinatorFacts(Facts); err != nil {
		return err
	}
	registerOnce.done = true
	return nil
}
