package records

import (
	"context"
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
)

func auditIntegrity(ctx context.Context, ledger any, target string, err error) error {
	var ce *cas.Error
	if !errors.As(err, &ce) {
		return err
	}
	l, ok := ledger.(interface {
		AppendAudit(context.Context, string, string, string, string) error
	})
	if !ok {
		return err
	}
	return errors.Join(err, l.AppendAudit(ctx, "artifacts", "blob.integrity.refused", target, "code="+ce.Code()+" reason="+ce.Reason))
}
