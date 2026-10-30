package report

// Purpose: the claim ledger interface (ADR 0032 decision 5, P7-AUTO-10). The
// status history of a commit is the ledger; claims are `pending` statuses on
// their own context so a late claim never overwrites a final result.
// Inputs:  a claim Status to post; a repository and sha to list.
// Outputs: the provider time of the response and the status history.
// Constraints: PostClaim is synchronous (no outbox); the reporter stays the
// only status writer; a provider error is returned, never turned into a state.

import (
	"context"
	"time"
)

// ClaimContext is the context claims are posted on.
const ClaimContext = Context + "/claim"

// LedgerStatus is one entry of a commit's status history.
type LedgerStatus struct {
	Context     string    `json:"context"`
	State       State     `json:"state"`
	Creator     string    `json:"creator"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// Ledger is the optional reporter extension that makes claims possible.
type Ledger interface {
	// PostClaim posts s (a pending status on ClaimContext) synchronously and
	// returns the provider's clock from the response.
	PostClaim(ctx context.Context, s Status) (providerNow time.Time, err error)
	// ListStatuses returns the full status history of a sha, newest first as
	// the provider gives it, with the provider's clock from the response.
	ListStatuses(ctx context.Context, owner, repo, sha string) (list []LedgerStatus, providerNow time.Time, err error)
}

// ClaimStatus builds the claim status for host and incarnation. The
// description never carries a path, token or repository content.
func ClaimStatus(owner, repo, sha, host, incarnation string) Status {
	return Status{Owner: owner, Repo: repo, Revision: sha, State: Pending, StatusContext: ClaimContext,
		Description: "claim " + host + " " + incarnation}
}
