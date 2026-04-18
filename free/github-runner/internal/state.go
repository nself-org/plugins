package internal

import "sync"

// RunnerState holds the in-memory state of the GitHub Actions self-hosted runner.
type RunnerState struct {
	mu         sync.Mutex
	Registered bool
	Running    bool
	Name       string
	Labels     string
	Org        string
	PID        int
}

// NewRunnerState creates an initial RunnerState.
func NewRunnerState() *RunnerState {
	return &RunnerState{}
}

// Snapshot returns a point-in-time copy of the state without holding the lock.
func (s *RunnerState) Snapshot() RunnerSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	return RunnerSnapshot{
		Registered: s.Registered,
		Running:    s.Running,
		Name:       s.Name,
		Labels:     s.Labels,
		Org:        s.Org,
		PID:        s.PID,
	}
}

// RunnerSnapshot is a lock-free point-in-time copy of RunnerState.
type RunnerSnapshot struct {
	Registered bool   `json:"registered"`
	Running    bool   `json:"running"`
	Name       string `json:"name"`
	Labels     string `json:"labels"`
	Org        string `json:"org"`
	PID        int    `json:"pid,omitempty"`
}
