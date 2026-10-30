package selftest

// Purpose: `runs prune --tidy [--apply]`: compose the run-history retention prune, the cache
// prune and the orphan job-root sweep, and measure the free-space target.
// Inputs:  the steps (each can list what it would remove and then remove exactly that),
// a free-space reader, the target in MB.
// Outputs: a Report listing every item; with Apply the free space before and after and
// whether the target was reached or why not.
// Constraints: without Apply nothing is removed, and the listed items are exactly what
// Apply removes (a step plans once; Apply performs that plan). A step whose plan failed
// is skipped and named in Errors; the other steps still run.

import (
	"context"
	"fmt"
)

// Item is one thing a tidy removes.
type Item struct {
	Step  string `json:"step"`
	ID    string `json:"id"`
	Bytes int64  `json:"bytes,omitempty"`
}

// Step is one part of a tidy. Plan lists what Apply would remove; Apply removes exactly the
// items Plan returned.
type Step struct {
	Name  string
	Plan  func(ctx context.Context) ([]Item, error)
	Apply func(ctx context.Context, items []Item) error
}

// Tidy runs the steps.
type Tidy struct {
	Steps    []Step
	FreeMB   func() (int64, error)
	TargetMB int64
}

// Report is the outcome of a tidy.
type Report struct {
	Apply        bool     `json:"apply"`
	Items        []Item   `json:"items"`
	FreeBeforeMB int64    `json:"free_before_mb"`
	FreeAfterMB  int64    `json:"free_after_mb"`
	TargetMB     int64    `json:"target_mb"`
	Reached      bool     `json:"reached"`
	Why          string   `json:"why,omitempty"`
	Errors       []string `json:"errors,omitempty"`
}

// Run plans every step and, with apply, removes what was planned.
func (t Tidy) Run(ctx context.Context, apply bool) (Report, error) {
	rep := Report{Apply: apply, TargetMB: t.TargetMB, Items: []Item{}}
	before, err := t.FreeMB()
	if err != nil {
		return rep, fmt.Errorf("free space not readable: %w", err)
	}
	rep.FreeBeforeMB, rep.FreeAfterMB = before, before
	planned := make([][]Item, len(t.Steps))
	for i, s := range t.Steps {
		items, perr := s.Plan(ctx)
		if perr != nil {
			rep.Errors = append(rep.Errors, s.Name+": "+perr.Error())
			continue
		}
		planned[i] = items
		rep.Items = append(rep.Items, items...)
	}
	if !apply {
		rep.Reached = before >= t.TargetMB
		rep.Why = fmt.Sprintf("report only: --apply would remove %d item(s)", len(rep.Items))
		return rep, nil
	}
	for i, s := range t.Steps {
		if len(planned[i]) == 0 {
			continue
		}
		if aerr := s.Apply(ctx, planned[i]); aerr != nil {
			rep.Errors = append(rep.Errors, s.Name+": "+aerr.Error())
		}
	}
	after, err := t.FreeMB()
	if err != nil {
		return rep, fmt.Errorf("free space not readable after the tidy: %w", err)
	}
	rep.FreeAfterMB, rep.Reached = after, after >= t.TargetMB
	if !rep.Reached {
		rep.Why = fmt.Sprintf("freed %d MB; %d MB free is %d MB below the %d MB target and nothing else is tidy to remove (history, cache and job roots are done)",
			after-before, after, t.TargetMB-after, t.TargetMB)
		if len(rep.Errors) > 0 {
			rep.Why += "; " + fmt.Sprintf("%d step(s) failed, see errors", len(rep.Errors))
		}
	}
	return rep, nil
}
