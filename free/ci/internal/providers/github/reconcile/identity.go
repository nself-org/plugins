package reconcile

// Purpose: keep an allowlisted repository name tied to the repository it was first seen as. The REST funnel
// follows same-host redirects, so a renamed or transferred repository answers under its old name, and a
// name that is later registered by someone else would otherwise be built as the operator's project.
// Inputs:  the repository object (or a run's repository) read by the coordinator's token.
// Outputs: nil, or an error wrapping ErrIdentity.
// Constraints: the API's full_name must equal the configured name (case-insensitive); the numeric id is
//          pinned in the store on first sight and must not change afterwards.

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrIdentity means the repository behind a configured name is not the one that was first seen.
var ErrIdentity = errors.New("reconcile: repository identity changed")

func (e *Engine) checkIdentity(ctx context.Context, repo string, id int64, fullName string) error {
	if fullName != "" && !strings.EqualFold(fullName, repo) {
		return fmt.Errorf("%w: %s answers as another repository (renamed or transferred)", ErrIdentity, repo)
	}
	if id <= 0 {
		return nil
	}
	var pin struct {
		ID int64 `json:"id"`
	}
	ok, err := e.getFact(ctx, repo, "identity", &pin)
	switch {
	case err != nil:
		return err
	case !ok:
		return e.putFact(ctx, repo, "identity", struct {
			ID int64 `json:"id"`
		}{id})
	case pin.ID != id:
		return fmt.Errorf("%w: %s changed repository id", ErrIdentity, repo)
	}
	return nil
}
