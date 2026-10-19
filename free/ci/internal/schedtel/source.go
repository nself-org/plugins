package schedtel

// Purpose: the planview.DecisionSource over the stored decision records, so
// `ci explain <run>/<job>` shows what the dispatcher decided and why.
// Inputs:  a run (pipeline) id and a job id or name.
// Outputs: a placement-plan document holding the latest decision of each
// matching job (retries of one job collapse to the newest attempt).
// Constraints: read only; the store is opened per call by the injected opener
// (the CLI has no long-lived store); no record is E692.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Opener returns a store and the function that releases it.
type Opener func(ctx context.Context) (*store.Store, func(), error)

// Source reads decision records from the store Open returns.
type Source struct{ Open Opener }

var _ planview.DecisionSource = Source{}

// Decision implements planview.DecisionSource.
func (s Source) Decision(ctx context.Context, run, job string) (model.PlacementPlan, error) {
	st, release, err := s.Open(ctx)
	if err != nil {
		return model.PlacementPlan{}, err
	}
	defer release()
	recs, err := st.TelDecisions(ctx, run, 0)
	if err != nil {
		return model.PlacementPlan{}, err
	}
	latest := map[string]store.TelDecision{} // job id -> newest decision (recs are oldest first)
	var order []string
	for _, d := range recs {
		if d.Job != job && d.JobName != job {
			continue
		}
		if _, seen := latest[d.Job]; !seen {
			order = append(order, d.Job)
		}
		latest[d.Job] = d
	}
	if len(order) == 0 {
		return model.PlacementPlan{}, &planview.Error{Code: planview.CodeExplainTarget,
			Message: fmt.Sprintf("run %q has no recorded decision for job %q; list recorded decisions with `nself ci runs decisions`", run, job)}
	}
	var out model.PlacementPlan
	for i, id := range order {
		var doc model.PlacementPlan
		if err = json.Unmarshal(latest[id].Doc, &doc); err != nil {
			return model.PlacementPlan{}, fmt.Errorf("schedtel: decision %s is unreadable: %w", id, err)
		}
		if i == 0 {
			out = doc
			continue
		}
		out.Jobs = append(out.Jobs, doc.Jobs...)
	}
	return out, nil
}
