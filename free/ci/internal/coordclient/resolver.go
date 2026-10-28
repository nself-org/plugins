package coordclient

// Purpose: the RemoteResolver of the coordinator client (P7-NODE-19, Epic D14): registered once through
// run.RegisterRemote, it answers `ci runs`, `ci runs cancel`, `ci logs` and `ci evidence` from the target
// Resolve picks, before any command opens the local store.
// Inputs:  a function that returns the Inputs of the current invocation (flag value, environment).
// Outputs: run.RemoteResolver plus the optional lister, canceller, log and evidence interfaces.
// Constraints: with no coordinator and no live local serve (InProcess) it answers "not mine" and does
//          nothing; with either one it is Active, so a failure (including "no such run") is an error the
//          command stops on rather than a fall back to the local store; a broken configuration is Active
//          too, so a typo in the coordinator address can never silently run the work on this machine;
//          warnings about ignored committed values are printed once per text.

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/detach"
	"github.com/nself-org/plugins/free/ci/internal/logs"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// Resolver is the registered RemoteResolver.
type Resolver struct {
	Inputs func() Inputs
	Warn   io.Writer // default os.Stderr

	mu     sync.Mutex
	warned map[string]bool
	cached *Client
	for_   Target
}

var (
	_ cirun.RemoteResolver       = (*Resolver)(nil)
	_ cirun.RemoteActive         = (*Resolver)(nil)
	_ cirun.RemoteLister         = (*Resolver)(nil)
	_ cirun.RemoteCanceller      = (*Resolver)(nil)
	_ cirun.RemoteLogReader      = (*Resolver)(nil)
	_ cirun.RemoteEvidenceReader = (*Resolver)(nil)
)

// Target resolves the destination of this invocation and prints new warnings.
func (r *Resolver) Target() (Target, error) {
	t, warns, err := Resolve(r.Inputs())
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.warned == nil {
		r.warned = map[string]bool{}
	}
	w := r.Warn
	if w == nil {
		w = os.Stderr
	}
	for _, line := range warns {
		if !r.warned[line] {
			r.warned[line] = true
			_, _ = io.WriteString(w, line+"\n")
		}
	}
	return t, err
}

// Client returns the client of the current target; ok is false for InProcess.
func (r *Resolver) Client() (*Client, Target, bool, error) {
	t, err := r.Target()
	if err != nil || !t.Active() {
		return nil, t, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cached == nil || r.for_ != t {
		c, err := New(t)
		if err != nil {
			return nil, t, true, err
		}
		r.cached, r.for_ = c, t
	}
	return r.cached, t, true, nil
}

// Active reports whether runs leave this process, or the configuration is too broken to tell.
func (r *Resolver) Active() bool {
	t, err := r.Target()
	return err != nil || t.Active()
}

func notFound(ref string) error {
	return clientapi.Failf(clientapi.ReasonNotFound, "no run %q on the coordinator", ref)
}

// Resolve answers where run ref lives; the empty ref is a listing, not a run.
func (r *Resolver) Resolve(ctx context.Context, ref string) (cirun.RunView, bool, error) {
	c, _, active, err := r.Client()
	if err != nil || !active || ref == "" {
		return cirun.RunView{}, false, err
	}
	it, ok, err := c.Get(ctx, ref)
	if err == nil && !ok {
		err = notFound(ref)
	}
	if err != nil {
		return cirun.RunView{}, false, err
	}
	return it.RunView, true, nil
}

// ListRuns lists the coordinator's runs.
func (r *Resolver) ListRuns(ctx context.Context) ([]cirun.RunView, bool, error) {
	c, _, active, err := r.Client()
	if err != nil || !active {
		return nil, false, err
	}
	items, err := c.List(ctx, 20)
	if err != nil {
		return nil, false, err
	}
	out := make([]cirun.RunView, len(items))
	for i, it := range items {
		out[i] = it.RunView
	}
	return out, true, nil
}

// CancelRun cancels ref on the coordinator.
func (r *Resolver) CancelRun(ctx context.Context, ref string) (int, bool, error) {
	c, _, active, err := r.Client()
	if err != nil || !active {
		return 0, false, err
	}
	n, err := c.Cancel(ctx, ref)
	return n, err == nil, err
}

// ReadLogs streams a job's stored log.
func (r *Resolver) ReadLogs(ctx context.Context, q cirun.RemoteLogQuery, fn func(cirun.RemoteLogLine) error) (bool, error) {
	c, _, active, err := r.Client()
	if err != nil || !active {
		return false, err
	}
	ok, err := c.Logs(ctx, clientapi.LogQuery{Run: q.Run, Job: q.Job, Attempt: q.Attempt, Follow: q.Follow}, func(l logs.Line) error {
		return fn(cirun.RemoteLogLine{TS: l.TS, Stream: l.Stream, Text: l.Text, Seq: l.Seq})
	})
	if err == nil && !ok {
		err = clientapi.Failf(clientapi.ReasonNotFound, "the coordinator has no stored log for job %q", q.Job)
	}
	return true, err
}

// ReadEvidence returns the signed evidence document of ref.
func (r *Resolver) ReadEvidence(ctx context.Context, ref string) (json.RawMessage, bool, error) {
	c, _, active, err := r.Client()
	if err != nil || !active {
		return nil, false, err
	}
	doc, err := c.Evidence(ctx, ref)
	return doc, err == nil, err
}

// FollowEvents uses the coordinator's durable event endpoint and reconnects by sequence.
func (r *Resolver) FollowEvents(ctx context.Context, ref string, from int64, w io.Writer) (int, error) {
	c, _, active, err := r.Client()
	if err != nil {
		return 0, err
	}
	if !active {
		return 0, notFound(ref)
	}
	rec, err := c.Events(ctx, ref, func(line []byte, rec detach.Record) error {
		if !rec.Final() && rec.Seq < from {
			return nil
		}
		_, err := w.Write(append(append([]byte(nil), line...), '\n'))
		return err
	})
	if rec.Err {
		return max(rec.Exit, 1), err
	}
	return 0, err
}
