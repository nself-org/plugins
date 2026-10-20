package sched

// Purpose: the cost class a runner is held to (Epic D11, P7-SCHED-07). The class the
// cost contributor priced the runner at wins over the class the runner claims for
// itself, so a rated provider cannot be made free by its own capability document.
// Inputs:  the world and a runner. Outputs: the class name.
// Constraints: pure; no contributor entry means the runner's own claim, as before.

// runnerClass is the cost class a runner is held to: the one the cost contributor priced
// it at, else the one it claims for itself. A rated provider cannot be made free by its
// own capability document.
func runnerClass(w World, r Runner) string {
	if c := w.Costs[r.ID].Class; c != "" {
		return c
	}
	return r.Class
}
