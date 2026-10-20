package netscope

// Purpose: the removal of an attempt's network and sidecar and the read-back of its denial events.
// Inputs:  the runtime, the attempt id and the sidecar's log.
// Outputs: Event values to the planner's sink; a once-only release func.
// Constraints: the release survives the attempt's cancellation (WithoutCancel), runs once, and reports what it
// could not read or remove as events instead of dropping the problem.

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"
)

// release reads the sidecar's denial events, then removes the sidecar and the network. It is idempotent.
func (p *Planner) release(rt Runtime, binary, attempt string) func(context.Context) {
	var once sync.Once
	return func(ctx context.Context) { once.Do(func() { p.releaseOnce(ctx, rt, binary, attempt) }) }
}

func (p *Planner) releaseOnce(ctx context.Context, rt Runtime, binary, attempt string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	log, err := rt.SidecarLog(ctx, binary, attempt)
	if err != nil {
		p.emit(attempt, Event{Reason: ReasonLogUnreadable, Detail: clip(err.Error(), 200)})
	}
	for _, e := range ParseEvents(strings.NewReader(string(log))) {
		p.emit(attempt, e)
	}
	if err := rt.Remove(ctx, binary, attempt); err != nil {
		p.emit(attempt, Event{Reason: ReasonCleanupFailed, Detail: clip(err.Error(), 200)})
	}
}

func (p *Planner) emit(attempt string, e Event) {
	e.Attempt = attempt
	if p.Events != nil {
		p.Events(attempt, e)
		return
	}
	_, _ = os.Stderr.Write(e.Encode())
}
