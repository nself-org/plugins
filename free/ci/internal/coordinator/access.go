package coordinator

// Purpose: read access to the coordinator for the listeners registered through RegisterListener
// (P7-NODE-19's user API): its store, its repo allowlist and its logger.
// Inputs:  none. Outputs: the values the Config holds.
// Constraints: accessors only; nothing here changes the coordinator.

import (
	"github.com/nself-org/plugins/free/ci/internal/serve"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Store is the coordinator's intake and run store.
func (c *Coordinator) Store() *store.Store { return c.cfg.Store }

// RepoAllowed reports whether project passes the coordinator's repo allowlist (an empty list allows none,
// as for a webhook).
func (c *Coordinator) RepoAllowed(project string) bool {
	return serve.RepoAllowed(project, c.cfg.AllowedRepos)
}

// Logf writes one line to the coordinator log.
func (c *Coordinator) Logf(format string, a ...any) { c.cfg.Logf(format, a...) }
