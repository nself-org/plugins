// Package mode decides the mode of a `nself ci serve` coordinator (P7-NODE-20, Epic D21, review F3) and
// provides the coordinator facts the world and the trust floor read (SCHED D22, TRUST D28).
package mode

// Purpose: the start check of `ci serve`: resolve the stored mode (personal by default, team only through
// --team), refuse a personal coordinator while anyone but the operator holds a grant, a token or a
// certificate, and refuse to start without the coordinator-facts provider.
// Inputs:  the coordinator store, the --team request and the actor name for the audit row.
// Outputs: a Decision {Mode, Switched}; an *Error with code E664 and reason personal_has_principals, or
//          reason facts_provider_missing.
// Constraints: fail closed. A mode row that cannot be read is team and the start is refused with the read
//          error; a principal count that cannot be read refuses the start. The mode never goes from team to
//          personal (the store enforces it). The count ignores the role of a grant and never counts the
//          operator's own `--self` tokens or certificates (store.NonSelfPrincipals). A message holds counts
//          and command names only, never a principal name or a token.

import (
	"context"
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// The two modes, as the store keeps them.
const (
	Personal = store.CoordinatorPersonal
	Team     = store.CoordinatorTeam
)

// CodeAuthz is E664 (authorization refused), registered by P7-NODE-19.
const CodeAuthz = "E664"

// Reason keys carried by Error.Reason.
const (
	ReasonPersonalHasPrincipals = "personal_has_principals"
	ReasonPrincipalsUnreadable  = "principals_unreadable"
	ReasonFactsProviderMissing  = "facts_provider_missing"
	ReasonModeUnreadable        = "mode_unreadable"
)

// Error is a start refusal.
type Error struct {
	Code, Reason, Message string
	Err                   error
}

func (e *Error) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

// Unwrap exposes the underlying store error, if any.
func (e *Error) Unwrap() error { return e.Err }

// ReasonOf returns the reason key of a mode error, or "".
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}

// Decision is the outcome of a successful start check.
type Decision struct {
	Mode     string // personal or team
	Switched bool   // this start switched the mode to team (one audit row was written)
}

// Start resolves the mode for one serve and applies the personal-mode principal rule.
func Start(ctx context.Context, st *store.Store, team bool, actor string) (Decision, error) {
	if st == nil {
		return Decision{Mode: Team}, &Error{Reason: ReasonModeUnreadable, Message: "refusing to start: no coordinator store"}
	}
	ch, err := st.CoordinatorModeEnsure(ctx, team, actor)
	if err != nil {
		return Decision{Mode: Team}, &Error{Reason: ReasonModeUnreadable, Err: err, Message: fmt.Sprintf("refusing to start: the coordinator mode cannot be stored or read (treated as team): %v", err)}
	}
	d := Decision{Mode: ch.Mode, Switched: ch.Switched}
	if d.Mode != Personal {
		return d, nil
	}
	n, err := st.NonSelfPrincipals(ctx)
	if err != nil {
		return Decision{Mode: Team}, &Error{Code: CodeAuthz, Reason: ReasonPrincipalsUnreadable, Err: err, Message: fmt.Sprintf("refusing to start in personal mode: the principals of this data dir cannot be read: %v", err)}
	}
	if n > 0 {
		return Decision{Mode: Team}, &Error{Code: CodeAuthz, Reason: ReasonPersonalHasPrincipals,
			Message: fmt.Sprintf("refusing to start in personal mode (personal_has_principals): %d grant, token or certificate issued to someone other than this operator exists; "+
				"start with `nself ci serve --team` to make this a team coordinator (permanent for this data dir), or revoke them "+
				"(`nself ci serve admin roles revoke <principal>`, `nself ci serve admin token revoke <id>`)", n)}
	}
	return d, nil
}

// RequireProvider refuses the start when the coordinator-facts provider is not registered, so the I15
// coordinator-host floor can never be silently off. registered is world.CoordinatorFactsRegistered in
// production; a test passes a stub.
func RequireProvider(registered func() bool) error {
	if registered == nil || !registered() {
		return &Error{Reason: ReasonFactsProviderMissing, Message: "refusing to start: no coordinator-facts provider is registered, so the coordinator-host trust floor would be off"}
	}
	return nil
}
