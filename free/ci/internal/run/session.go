package run

// Purpose: the per-run state CreateAttempt needs and the run loop shares: the
// store, the coordinator identity, the host, the executor and the job table.
// Inputs:  built by Coordinator.Run; carried in the context.
// Outputs: SessionFrom(ctx) for CreateAttempt and the hooks that call it
// (P7-CI-40 retry, P7-CI-41 reproduce, P7-CI-52 ad hoc).
// Constraints: a Session is safe for concurrent use; job identity is the
// *model.Job pointer in the persisted pipeline (index-aligned with the plan),
// with a content fallback for a copied job.

import (
	"context"
	"os"
	"reflect"
	"regexp"
	"sync"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Session is the state of one run.
type Session struct {
	Store       *store.Store
	Coordinator store.Coordinator
	Exec        *ciexec.Executor
	RunID       string
	Revision    string
	Host        string
	Home        string
	Poll        time.Duration
	Capacity    func() ciexec.Capacity
	Admission   ciexec.AdmissionPolicy
	// Plan is the run's plan and Record its Options.Record: the dispatch hook maps both
	// into the scheduler job (P7-AUTO-28); a Session built without them still places.
	Plan   *plan.Plan
	Record bool
	// LocalOnly says a job cannot be run on a node yet (nil = none): the dispatch hook keeps it on this machine.
	LocalOnly func(model.Job) bool
	// Clock is the time the placement world is built at (nil = time.Now); tests inject it so a plan and a run can be compared.
	Clock       func() time.Time
	Interactive bool      // a person waits at a terminal: only then does the implicit foreground queue cap apply
	PolicyRun   RunValues // the per-run policy values of the invocation; the dispatcher resolves its placement policy with them

	mu    sync.Mutex
	jobs  map[*model.Job]string // job row id by job pointer
	byID  map[string]model.Job  // job copy by row id (content fallback)
	local map[string]bool       // attempts admitted on this host
	slots slotPool              // execution slots of concurrent attempts
}

type sessionKey struct{}

// WithSession returns a context CreateAttempt can read.
func WithSession(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, sessionKey{}, s)
}

// SessionFrom returns the session of ctx, or nil.
func SessionFrom(ctx context.Context) *Session {
	s, _ := ctx.Value(sessionKey{}).(*Session)
	return s
}

func newSession() *Session {
	return &Session{jobs: map[*model.Job]string{}, byID: map[string]model.Job{}, local: map[string]bool{}, Poll: 50 * time.Millisecond}
}

// bind records the row id of a persisted job.
func (s *Session) bind(job *model.Job, rowID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job] = rowID
	s.byID[rowID] = *job
}

// jobRow finds the row id of job: by pointer, then by unique content.
func (s *Session) jobRow(job *model.Job) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.jobs[job]; ok {
		return id, true
	}
	found, n := "", 0
	for id, j := range s.byID {
		if reflect.DeepEqual(j, *job) {
			found, n = id, n+1
		}
	}
	return found, n == 1
}

// now is the session's clock.
func (s *Session) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

// jobRows lists the row ids of the run's jobs.
func (s *Session) jobRows() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.byID))
	for id := range s.byID {
		out = append(out, id)
	}
	return out
}

// safeID makes a store id usable where the executor wants [A-Za-z0-9_.-].
var unsafeID = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func safeID(id string) string { return unsafeID.ReplaceAllString(id, "-") }

// background returns a context that outlives cancellation of ctx (cleanup
// writes after Ctrl-C), bounded in time.
func background(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
}

func hostName() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "localhost"
	}
	return safeID(h)
}
