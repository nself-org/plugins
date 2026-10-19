package fairq

// Purpose: the error code of the fair queue (Epic D17): E694, a project whose
// queue is full. The dispatcher raises it at submission and carries the
// expected wait (internal/dispatch/backpressure.go).
// Constraints: registered through model.Register, never a panic.

import "github.com/nself-org/plugins/free/ci/internal/model"

// CodeBackpressure is the code of a submission refused because the project's queue is full.
const CodeBackpressure = "E694"

func init() {
	model.Register(model.Code{ID: CodeBackpressure, Class: "usage", Summary: "the project's job queue is full", Fix: "Wait for queued jobs to finish (the error carries the expected wait), cancel runs you no longer need, or raise sched.queue.max_depth"})
}
