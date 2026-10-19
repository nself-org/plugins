package evidence

// Purpose: the hook behind `ci evidence --pipeline <name>`: the overdue
// report of a named pipeline (P7-CI-02 registers the reporter; this package
// only dispatches). Exit 0 none overdue, 10 at least one overdue.
// Constraints: a registration happens at init; a second registration of a
// name replaces the first; no reporter registered is E631, never a guess.

import (
	"context"
	"sort"
	"sync"
)

// PipelineReport is what a reporter returns for one pipeline.
type PipelineReport struct {
	Pipeline string `json:"pipeline"`
	// Overdue is true when the pipeline has no current evidence within max_age.
	Overdue bool `json:"overdue"`
	// Detail is the reporter's own JSON-shaped description.
	Detail any `json:"detail,omitempty"`
}

// PipelineReporter reports on named pipelines.
type PipelineReporter interface {
	Report(ctx context.Context, name string) (PipelineReport, error)
}

var (
	reporterMu sync.Mutex
	reporters  = map[string]PipelineReporter{}
)

// RegisterPipelineReporter makes r answer for the pipeline called name; the
// name "*" answers for every pipeline no other reporter claims.
func RegisterPipelineReporter(name string, r PipelineReporter) {
	reporterMu.Lock()
	defer reporterMu.Unlock()
	reporters[name] = r
}

// ReportPipeline asks the registered reporter about one pipeline.
func ReportPipeline(ctx context.Context, name string) (PipelineReport, error) {
	reporterMu.Lock()
	r, ok := reporters[name]
	if !ok {
		r, ok = reporters["*"]
	}
	reporterMu.Unlock()
	if !ok {
		return PipelineReport{}, fail(CodeNone, ExitFails, "no pipeline reporter is registered for %q", name)
	}
	return r.Report(ctx, name)
}

// ReporterNames lists the registered names, sorted.
func ReporterNames() []string {
	reporterMu.Lock()
	defer reporterMu.Unlock()
	out := make([]string, 0, len(reporters))
	for n := range reporters {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
