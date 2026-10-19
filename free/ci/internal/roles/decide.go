package roles

// Purpose: the one decision path behind Authorize and AuthorizeAction: check, then write the audit row.
// Inputs:  a decision.
// Outputs: nil or the E687 error; an audit row "authorize" with result=allow|deny for audited decisions.
// Constraints: a denial is audited too; if the row cannot be written the decision fails closed and the
//          caller sees the audit error; the row names the actor, the role or action, the project and the
//          reason key, never a token.

import (
	"context"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/audit"
)

func (a Actor) decide(ctx context.Context, d decision) error {
	err := d.forced
	if err == nil {
		err = a.Can(d.min, d.project)
	}
	if err == nil && d.needProject && d.project == "" {
		err = fail(ReasonProject, "%s needs a project", d.target)
	}
	if err == nil && d.nonAuthor {
		author, _ := ctx.Value(authKey{}).(string)
		switch {
		case author == "":
			err = fail(ReasonNoAuthor, "the author of the run is unknown, so it cannot be approved")
		case author == a.ID:
			err = fail(ReasonSelfApproval, "%s cannot approve its own run", a.Label())
		}
	}
	if !d.audited || a.svc == nil {
		return err
	}
	result, reason := "allow", ""
	if err != nil {
		result, reason = "deny", " reason="+ReasonOf(err)
	}
	ev := audit.Event{Actor: a.Label(), Action: d.action, Target: d.target,
		Detail: fmt.Sprintf("result=%s role=%s project=%s%s", result, d.min, d.project, reason)}
	if aerr := a.svc.Log.Append(ctx, ev); aerr != nil {
		return fmt.Errorf("audit row not written, decision refused: %w", aerr)
	}
	return err
}
