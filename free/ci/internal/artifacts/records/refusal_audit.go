package records

import (
	"context"
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

// AuditRefusal records stable metadata only: refusal messages can contain secret
// values or hostile paths. Unrelated I/O faults and invisible IDs are not refusals.
func AuditRefusal(ctx context.Context, ledger any, action, target string, err error) error {
	code, reason := "", ""
	var re *Error
	var pe *authz.PathError
	switch {
	case errors.As(err, &re) && re.ID == CodeArtifact:
		code, reason = re.ID, re.Reason
	case errors.As(err, &pe):
		code, reason = authz.CodeArtifactPath, string(pe.Reason)
	default:
		return err
	}
	l, ok := ledger.(interface {
		AppendAudit(context.Context, string, string, string, string) error
	})
	if !ok {
		return err
	}
	scan := "unknown"
	if reason == ReasonSecretMatch {
		scan = "refused"
	}
	return errors.Join(err, l.AppendAudit(ctx, "artifacts", action, target, "code="+code+" reason="+reason+" scan="+scan))
}
