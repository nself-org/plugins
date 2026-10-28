package hosted

// Purpose: the AttemptFacts source of the provider driver (the handoff of P7-HOST-16): which
//          repository, project, runner label, trust class, fetch ref and timeout a leased hosted
//          attempt runs with.
// Inputs:  the leased node (its labels name repo, project and runner label) and the lease (the job
//          timeout); the optional Attempts source for the trust class, the attempt number and the
//          fetch ref of a fork pull request head.
// Outputs: runtime.AttemptFacts; installed on the default driver from init().
// Constraints: with no Attempts source the trust class is "untrusted" (so work dispatches on the
//          untrusted ref and the attempt number is 1): the safe default never widens trust. The
//          repository and label come only from the node the scheduler placed the job on, never from
//          the job. A node without a repository is E708.

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
)

// AttemptMeta is what the coordinator knows about an attempt beyond its lease.
type AttemptMeta struct {
	N        int
	Trust    string
	FetchRef string
}

// Attempts looks an attempt up; ok is false when it is unknown.
type Attempts func(ctx context.Context, attemptID string) (AttemptMeta, bool, error)

var attemptSource struct {
	sync.RWMutex
	fn Attempts
}

// UseAttempts installs the source of attempt metadata (the coordinator start does this once). nil removes it.
func UseAttempts(fn Attempts) {
	attemptSource.Lock()
	defer attemptSource.Unlock()
	attemptSource.fn = fn
}

func labelValue(labels []string, prefix string) string {
	for _, l := range labels {
		if v, ok := strings.CutPrefix(l, prefix); ok {
			return v
		}
	}
	return ""
}

// validRepo checks the repository label with the check of the node's provider (GitLab accepts a
// nested project path); a provider with none registered is held to GitHub's owner/name.
func validRepo(provider, repo string) bool {
	if check, ok := runtime.RepoCheck(provider); ok {
		return check(repo)
	}
	_, ok := parseRepo(repo)
	return ok
}

// Facts implements runtime.FactsSource.
func Facts(ctx context.Context, node model.Capability, l protocol.Lease) (runtime.AttemptFacts, error) {
	repo := labelValue(node.Identity.Labels, LabelRepo)
	if !validRepo(node.Identity.Provider, repo) {
		return runtime.AttemptFacts{}, refuse("node.repo")
	}
	// The lease epoch counts the times this attempt was handed out again after a loss: it is the
	// attempt number the journal fences on, so a re-issued lease dispatches a new run and cancels
	// the old one instead of adopting it. An Attempts source may name a number of its own.
	f := runtime.AttemptFacts{N: max(1, int(l.Epoch)), Project: labelValue(node.Identity.Labels, LabelProject), Repo: repo,
		Label: labelValue(node.Identity.Labels, LabelRunner), Trust: "untrusted"}
	if d, err := time.ParseDuration(l.Job.Timeout); err == nil && d > 0 {
		f.TimeoutMs = d.Milliseconds()
	}
	attemptSource.RLock()
	fn := attemptSource.fn
	attemptSource.RUnlock()
	if fn == nil {
		return f, nil
	}
	m, ok, err := fn(ctx, l.AttemptID)
	if err != nil {
		return runtime.AttemptFacts{}, err
	}
	if ok {
		if m.N > f.N {
			f.N = m.N
		}
		if m.Trust != "" {
			f.Trust = m.Trust
		}
		f.FetchRef = m.FetchRef
	}
	return f, nil
}
