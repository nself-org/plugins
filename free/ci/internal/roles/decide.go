package roles

// Purpose: the one decision path behind Authorize and AuthorizeAction: re-read the actor's state, check,
// then write the audit row.
// Inputs:  a decision.
// Outputs: nil or the E687 error; an audit row "authorize" with result=allow|deny for audited decisions.
// Constraints: a denial is audited too; if the row cannot be written the decision fails closed and the
//          caller sees the audit error; the row names the actor, the role or action, the project (quoted)
//          and the reason key, never a token; the non-author rule binds every identity except the local
//          operator, who is the only person of a PERSONAL coordinator.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/audit"
)

// principalOf reduces an audit label or a bare id to the principal id, so "token:tok-1:bob",
// "user:bob", "operator:cli:bob" and "bob" all name the same person.
func principalOf(label string) string {
	s := strings.ToLower(strings.TrimSpace(label))
	s = strings.TrimPrefix(s, "operator:")
	s = strings.TrimPrefix(s, "user:")
	if strings.HasPrefix(s, "token:") {
		s = s[strings.LastIndex(s, ":")+1:]
	}
	return strings.TrimPrefix(s, "cli:")
}

func (a Actor) decide(ctx context.Context, d decision) error {
	var err error
	if a.svc != nil && a.id != "" && a.kind != "" {
		if a, err = a.refresh(ctx); err != nil && ReasonOf(err) == "" {
			return err // the store could not be read: fail closed without a decision row
		}
	}
	if err == nil {
		err = d.forced
	}
	if err == nil {
		err = a.Can(d.min, d.project)
	}
	if err == nil && d.needProject && d.project == "" {
		err = fail(ReasonProject, "%s needs a project", d.target)
	}
	if err == nil && d.nonAuthor && a.kind != KindOperator {
		author, _ := ctx.Value(authKey{}).(string)
		switch {
		case author == "":
			err = fail(ReasonNoAuthor, "the author of the run is unknown, so it cannot be approved")
		case principalOf(author) == principalOf(a.id):
			err = fail(ReasonSelfApproval, "%s cannot approve its own run", a.Label())
		}
	}
	if !d.audited || a.svc == nil {
		return err
	}
	if a.svc.Log == nil {
		return errors.New("no audit log configured, decision refused")
	}
	result, reason := "allow", ""
	if err != nil {
		result, reason = "deny", " reason="+ReasonOf(err)
	}
	ev := audit.Event{Actor: a.Label(), Action: d.action, Target: d.target,
		Detail: fmt.Sprintf("result=%s role=%s project=%q%s", result, d.min, d.project, reason)}
	if aerr := a.svc.Log.Append(ctx, ev); aerr != nil {
		return fmt.Errorf("audit row not written, decision refused: %w", aerr)
	}
	return err
}

// refresh re-reads what the actor's role rests on. A grant, token or certificate revoked since the actor
// was resolved ends its access here; a changed role or project list applies at once.
func (a Actor) refresh(ctx context.Context) (Actor, error) {
	switch a.kind {
	case KindUser:
		if !a.notAfter.IsZero() && !a.svc.Now().Before(a.notAfter) {
			return a, fail(ReasonBadCert, "the certificate expired")
		}
		if a.serial != "" {
			denied, err := a.svc.St.PKIIsDenied(ctx, a.serial)
			if err != nil {
				return a, err
			}
			if denied {
				return a, fail(ReasonBadCert, "the certificate is revoked")
			}
		}
		g, ok, err := a.svc.St.GrantGet(ctx, a.id)
		if err != nil {
			return a, err
		}
		if !ok {
			return a, fail(ReasonNoGrant, "user %s holds no role", a.id)
		}
		a.role, a.projects = Role(g.Role), g.Projects
	case KindToken:
		t, ok, err := a.svc.St.TokenByID(ctx, a.tokenID)
		if err != nil {
			return a, err
		}
		if err = a.svc.tokenState(t, ok); err != nil {
			return a, err
		}
		a.role, a.projects = Role(t.Role), t.Projects
	}
	return a, nil
}
