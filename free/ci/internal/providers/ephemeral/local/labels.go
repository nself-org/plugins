package local

// Purpose: the small pure helpers of the provider: intent validation, adoption of a sandbox the
//          runtime already holds for an intent, resource copies and error classification.
// Inputs:  burst.Intent, backend Found values, backend errors.
// Outputs: validated intents (an E698 *Error otherwise), copies that share no map with the ledger.
// Constraints: no I/O; labels are copied, never aliased, so a caller cannot edit the ledger.

import (
	"context"
	"errors"
	"maps"

	"github.com/nself-org/plugins/free/ci/internal/burst"
)

// classify turns a backend failure into the error class the lifecycle retries on: a cancelled caller
// stays a context error, anything else is an unavailable provider (E698).
func classify(ctx context.Context, err error) error {
	var e *Error
	switch {
	case errors.As(err, &e):
		return err
	case ctx.Err() != nil && errors.Is(err, ctx.Err()):
		return err
	}
	return unavailable(ReasonBackend, err, "%v", err)
}

func checkIntent(in burst.Intent) error {
	if !tokenRe.MatchString(in.Token) || in.Labels[intentKey] != in.Token || in.Labels[roleKey] != roleValue || in.Labels[coordKey] == "" {
		return unavailable(ReasonBadRequest, nil, "an intent needs a token of 1-%d letters or digits and the role, coordinator_id and intent labels", burst.MaxLabelValue)
	}
	for k, v := range in.Labels {
		if len(v) > burst.MaxLabelValue || len(k) == 0 {
			return unavailable(ReasonBadRequest, nil, "label %q is empty or longer than %d characters", k, burst.MaxLabelValue)
		}
	}
	return nil
}

func adoptable(found []Found, in burst.Intent) (Found, bool) {
	for _, f := range found {
		if f.Labels[intentKey] == in.Token && burst.Mine(f.Labels, in.Labels[coordKey]) {
			return f, true
		}
	}
	return Found{}, false
}

func resFromFound(f Found) burst.Resource {
	return burst.Resource{ID: f.ID, Name: namePrefix + f.ID, Labels: maps.Clone(f.Labels), CreatedAt: f.CreatedAt}
}

func cloneRes(r burst.Resource) burst.Resource {
	r.Labels = maps.Clone(r.Labels)
	return r
}
