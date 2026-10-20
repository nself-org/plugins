package coordinator

// Purpose: stops new intake claims (P7-NODE-20, Epic D7 drain): once paused, the dispatch loop claims no
//          further row, while rows already running finish or are handed back by the drain.
// Inputs:  none. Outputs: PauseIntake, IntakePaused. Constraints: idempotent; never reversed in a process.

// PauseIntake stops the dispatch loop from claiming new rows. It does not touch rows already running.
func (c *Coordinator) PauseIntake() { c.paused.Store(true) }

// IntakePaused reports whether PauseIntake ran.
func (c *Coordinator) IntakePaused() bool { return c.paused.Load() }

// Stop makes the Coordinator a drain.Intake.
func (c *Coordinator) Stop() { c.PauseIntake() }
