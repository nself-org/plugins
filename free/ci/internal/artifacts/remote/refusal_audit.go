package remote

import (
	"context"
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
)

func (l *Lease) auditRefusal(ctx context.Context, sha string, err error) {
	var te *transfer.Error
	var re *records.Error
	var ce *cas.Error
	var pe *authz.PathError
	switch {
	case errors.As(err, &te):
		l.refuse(ctx, sha, te)
	case errors.As(err, &ce):
		l.auditCode(ctx, sha, ce.Code(), ce.Reason)
	case errors.As(err, &re):
		l.auditCode(ctx, sha, re.Code(), re.Reason)
	case errors.As(err, &pe):
		l.auditCode(ctx, sha, authz.CodeArtifactPath, string(pe.Reason))
	}
}
