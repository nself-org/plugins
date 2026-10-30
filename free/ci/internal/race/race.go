package race

// Purpose: drive one race. Every sibling is created through Attempts.Create
// (run.CreateAttempt behind it, the only attempt creator), one per chosen
// runner. The first conclusive result wins; every other live sibling gets one
// lease cancel (NODE-23 kills its process group on the node); the race waits
// until every sibling has ended, then posts exactly one Evidence.
// Inputs:  a Choice from Select, the Attempts and the Poster.
// Outputs: the Evidence that was posted.
// Constraints: cancellation is idempotent and fenced by the sibling's lease
// epoch; a sibling that ends after the winner is superseded (or flaky when its
// conclusive result differs), never a second result; a sibling admitted on a
// runner another sibling holds is cancelled at once; no sibling posts.

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// ErrNotRaced is returned when the job cannot be raced: the caller runs one attempt.
var ErrNotRaced = errors.New("race: job not raced")

// Attempts creates, awaits and cancels sibling attempts.
type Attempts interface {
	// Create makes attempt n of the job, placed on runner.
	Create(ctx context.Context, n int, runner string) (Sibling, error)
	// Wait blocks until the sibling has ended.
	Wait(ctx context.Context, s Sibling) Result
	// Cancel asks the sibling's lease to stop, fenced by s.Epoch. A repeat or a
	// cancel that lost to the result commit is not an error.
	Cancel(ctx context.Context, s Sibling, reason string) error
}

// Poster records and posts the one result of a race.
type Poster interface {
	Post(ctx context.Context, e Evidence) error
}

// Spec is one race.
type Spec struct {
	Job    string // the job key, for the evidence
	Choice Choice
	First  int // attempt number of the first sibling (>= 1)
}

type ended struct {
	i int
	r Result
}

// Run races the siblings of spec and posts one Evidence.
func Run(ctx context.Context, spec Spec, a Attempts, p Poster) (Evidence, error) {
	ev := Evidence{Job: spec.Job, Width: len(spec.Choice.Runners), Losers: []Loser{}, Excluded: spec.Choice.Excluded}
	if !spec.Choice.Raceable() {
		return ev, fmt.Errorf("%w: %s", ErrNotRaced, spec.Choice.Reason)
	}
	sibs := create(ctx, spec, a, &ev)
	if len(sibs) == 0 {
		return ev, fmt.Errorf("%w: no sibling was admitted", ErrNotRaced)
	}
	done := make(chan ended, len(sibs))
	for i, s := range sibs {
		go func() { done <- ended{i, a.Wait(ctx, s)} }()
	}
	winner, first := -1, -1
	results := make([]*Result, len(sibs))
	asked := make([]bool, len(sibs))
	cancelRest := func(reason string) {
		for i, s := range sibs {
			if i != winner && results[i] == nil && !asked[i] {
				asked[i] = true
				if err := a.Cancel(context.WithoutCancel(ctx), s, reason); err != nil {
					ev.Notes = append(ev.Notes, fmt.Sprintf("cancel %s: %v", s.AttemptID, err))
				}
			}
		}
	}
	for range sibs {
		var e ended
		select {
		case e = <-done:
		case <-ctx.Done():
			cancelRest("run cancelled")
			e = <-done
		}
		r := e.r
		results[e.i] = &r
		if first < 0 {
			first = e.i
		}
		if winner < 0 && Conclusive(r) {
			winner = e.i
			cancelRest("race lost to " + sibs[e.i].AttemptID)
		}
	}
	ev.Result = *results[first]
	if winner >= 0 {
		w := sibs[winner]
		ev.Winner, ev.Result = &w, *results[winner]
	}
	for i, s := range sibs {
		if i == winner {
			continue
		}
		l := Loser{Sibling: s, Result: results[i], Disposition: dispose(winner >= 0, asked[i], *results[i], ev.Result)}
		ev.Flaky = ev.Flaky || l.Disposition == Flaky
		ev.Losers = append(ev.Losers, l)
	}
	slices.SortFunc(ev.Losers, func(x, y Loser) int { return x.N - y.N })
	if ctx.Err() != nil {
		return ev, ctx.Err() // a cancelled run posts nothing from the race
	}
	if err := p.Post(context.WithoutCancel(ctx), ev); err != nil {
		return ev, err
	}
	return ev, nil
}

// create makes one sibling per chosen runner. A sibling admitted on a runner
// another sibling already holds is cancelled and left out.
func create(ctx context.Context, spec Spec, a Attempts, ev *Evidence) []Sibling {
	var sibs []Sibling
	held := map[string]bool{}
	n := max(spec.First, 1)
	for _, runner := range spec.Choice.Runners {
		s, err := a.Create(ctx, n, runner)
		n++
		if err != nil {
			ev.Notes = append(ev.Notes, fmt.Sprintf("sibling on %s not admitted: %v", runner, err))
			continue
		}
		if held[s.RunnerID] {
			_ = a.Cancel(context.WithoutCancel(ctx), s, ReasonSibling)
			ev.Losers = append(ev.Losers, Loser{Sibling: s, Disposition: Cancelled})
			ev.Notes = append(ev.Notes, fmt.Sprintf("sibling %s admitted on %s, which a sibling holds: cancelled", s.AttemptID, s.RunnerID))
			continue
		}
		held[s.RunnerID] = true
		sibs = append(sibs, s)
	}
	return sibs
}

// dispose names how a non-winning sibling ended.
func dispose(won, asked bool, r, result Result) string {
	switch {
	case !won:
		return Lost
	case Conclusive(r) && (r.State != result.State || r.FailureClass != result.FailureClass):
		return Flaky
	case asked && r.State == StateCancelled:
		return Cancelled
	}
	return Superseded
}
