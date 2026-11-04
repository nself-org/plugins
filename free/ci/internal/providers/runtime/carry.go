package runtime

// Purpose: which attempts a driver Run of this process is carrying (D-1297). The coordinator's hosted poll
//          loop and the driver both watch the same run; when the run finishes, the carrier collects the
//          evidence and reports the result, which moves the attempt on. If the poll loop saw "finished"
//          first, the lease engine would finalize the attempt and the carrier's real result would then be
//          refused. So the lease profile reports a finished run that is still carried as running: the lease
//          stays alive until the carrier reports, and the poll loop never races it.
// Inputs:  attempt ids, from Driver.Run.
// Outputs: hold marks an attempt carried until the returned func runs; carried reports it.
// Constraints: counted, so two carriers of one attempt (a restart racing a late one) both have to end;
//          a finished run nobody carries is still finished, so the engine finalizes it as before.

import "sync"

// carrySet counts the Runs carrying each attempt. The zero value is ready to use.
type carrySet struct {
	mu sync.Mutex
	n  map[string]int
}

// hold marks attemptID carried; the returned func releases it.
func (c *carrySet) hold(attemptID string) (release func()) {
	c.mu.Lock()
	if c.n == nil {
		c.n = map[string]int{}
	}
	c.n[attemptID]++
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.n[attemptID]--; c.n[attemptID] <= 0 {
			delete(c.n, attemptID)
		}
	}
}

// carried reports whether a Run is carrying attemptID.
func (c *carrySet) carried(attemptID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n[attemptID] > 0
}
