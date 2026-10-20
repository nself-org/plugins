package run

// Purpose: the remote side of the read commands (P7-NODE-19, Epic D14, CI delta 9): `ci runs`, `ci runs
// cancel`, `ci logs` and `ci evidence` ask the registered RemoteResolvers before they open the local
// store. A resolver that also implements the optional interfaces below can answer more than "where does
// this run live": it lists, cancels, streams logs and returns the signed evidence of a run on a
// coordinator (or on a local serve that owns the store).
// Inputs:  the registered Hooks.Remote resolvers.
// Outputs: ResolveRemote, ListRemote, CancelRemote, LogsRemote, EvidenceRemote; each returns handled=true
//          when a resolver answered.
// Constraints: a resolver whose target is Active (a coordinator or a local serve is configured) turns
//          every failure into an error the caller must stop on: the command never falls back to the local
//          store then, because a local serve holds the store and a coordinator owns the run; a resolver
//          that is not Active (or does not say) is skipped on failure, as before; nothing here prints.

import (
	"context"
	"encoding/json"
)

// RemoteActive may be implemented by a RemoteResolver: true when runs leave this process.
type RemoteActive interface{ Active() bool }

// RemoteLister lists the runs a coordinator knows.
type RemoteLister interface {
	ListRuns(ctx context.Context) ([]RunView, bool, error)
}

// RemoteCanceller cancels a run on a coordinator and returns how many attempts it asked to stop.
type RemoteCanceller interface {
	CancelRun(ctx context.Context, ref string) (n int, handled bool, err error)
}

// RemoteLogLine is one stored log line.
type RemoteLogLine struct {
	TS, Stream, Text string
	Seq              int64
}

// RemoteLogQuery selects one job's stored log.
type RemoteLogQuery struct {
	Run, Job string
	Attempt  int
	Follow   bool
}

// RemoteLogReader streams stored log lines from a coordinator.
type RemoteLogReader interface {
	ReadLogs(ctx context.Context, q RemoteLogQuery, fn func(RemoteLogLine) error) (handled bool, err error)
}

// RemoteEvidenceReader returns a run's evidence document exactly as the coordinator signed it.
type RemoteEvidenceReader interface {
	ReadEvidence(ctx context.Context, ref string) (doc json.RawMessage, handled bool, err error)
}

// RemoteFatal reports whether a failure of r must stop the command (r is an active coordinator target).
func RemoteFatal(r RemoteResolver) bool {
	a, ok := r.(RemoteActive)
	return ok && a.Active()
}

// ResolveRemote asks the resolvers about one run.
func ResolveRemote(ctx context.Context, ref string) (RunView, bool, error) {
	for _, r := range Registered.Remote {
		v, ok, err := r.Resolve(ctx, ref)
		switch {
		case err != nil && RemoteFatal(r):
			return RunView{}, false, err
		case err == nil && ok:
			return v, true, nil
		}
	}
	return RunView{}, false, nil
}

// ListRemote lists the runs of the active coordinator.
func ListRemote(ctx context.Context) ([]RunView, bool, error) {
	for _, r := range Registered.Remote {
		if l, ok := r.(RemoteLister); ok {
			if v, handled, err := l.ListRuns(ctx); handled || err != nil && RemoteFatal(r) {
				return v, handled, err
			}
		}
	}
	return nil, false, nil
}

// CancelRemote cancels a run on the active coordinator.
func CancelRemote(ctx context.Context, ref string) (int, bool, error) {
	for _, r := range Registered.Remote {
		if c, ok := r.(RemoteCanceller); ok {
			if n, handled, err := c.CancelRun(ctx, ref); handled || err != nil && RemoteFatal(r) {
				return n, handled, err
			}
		}
	}
	return 0, false, nil
}

// LogsRemote streams a job's stored log from the active coordinator.
func LogsRemote(ctx context.Context, q RemoteLogQuery, fn func(RemoteLogLine) error) (bool, error) {
	for _, r := range Registered.Remote {
		if l, ok := r.(RemoteLogReader); ok {
			if handled, err := l.ReadLogs(ctx, q, fn); handled || err != nil && RemoteFatal(r) {
				return handled, err
			}
		}
	}
	return false, nil
}

// EvidenceRemote returns a run's signed evidence document from the active coordinator.
func EvidenceRemote(ctx context.Context, ref string) (json.RawMessage, bool, error) {
	for _, r := range Registered.Remote {
		if e, ok := r.(RemoteEvidenceReader); ok {
			if d, handled, err := e.ReadEvidence(ctx, ref); handled || err != nil && RemoteFatal(r) {
				return d, handled, err
			}
		}
	}
	return nil, false, nil
}
