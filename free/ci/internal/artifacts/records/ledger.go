package records

// Purpose: the storage contract of the artifact records. *store.Store
// implements it (internal/store/cache_artifacts*.go); tests use a fake.
// Inputs:  attempt ids and lease epochs, Records, Decls.
// Outputs: Record values and refusals.
// Constraints: every write re-reads the attempt state and epoch inside one
// transaction; a stale capability is an E711 refusal with one audit row; a
// record is per attempt (the pair attempt+name is unique, so two attempts
// that produce the same path create two records); only expires_at and the
// missing flag change after commit.

import (
	"context"
	"time"
)

// Filter selects records for list. Empty fields match everything.
type Filter struct {
	Project, Run, Attempt, Name string
	Kind                        Kind
	Limit                       int
}

// PruneSpec says what prune may remove.
type PruneSpec struct {
	Project   string // only this project's records ("" = every project: the automatic prune)
	Now       time.Time
	Release   bool          // include release-kind records
	OlderThan time.Duration // with Release: also release records created before Now-OlderThan
	DryRun    bool
}

// BlobRef names a stored blob some record still needs.
type BlobRef struct{ Project, SHA256 string }

// Ledger is the record store.
type Ledger interface {
	// ArtifactFacts returns the stored facts of the attempt.
	ArtifactFacts(ctx context.Context, attempt string) (Facts, error)
	// ArtifactDeclare stores the declarations (state running, current epoch).
	ArtifactDeclare(ctx context.Context, attempt string, epoch int64, decls []Decl) error
	// ArtifactDecls returns the declarations of the attempt in order.
	ArtifactDecls(ctx context.Context, attempt string) ([]Decl, error)
	// ArtifactReserve claims bytes of the store for the attempt before any is written.
	ArtifactReserve(ctx context.Context, attempt string, epoch int64, bytes int64, pol Policy) error
	// ArtifactStage stores staged records (state running, epoch as declared).
	ArtifactStage(ctx context.Context, attempt string, epoch int64, recs []Record) error
	// ArtifactStaged returns the attempt's records that are not committed yet.
	ArtifactStaged(ctx context.Context, attempt string) ([]Record, error)
	// ArtifactCommit flips the verified staged records to committed (state
	// finalizing, current epoch), checks the caps and returns the 80 % warning.
	ArtifactCommit(ctx context.Context, attempt string, epoch int64, verified []Record, pol Policy) (string, error)
	ArtifactList(ctx context.Context, f Filter) ([]Record, error)
	ArtifactGet(ctx context.Context, id string) (Record, bool, error)
	// ArtifactSetMissing records whether a committed record's blob is gone.
	ArtifactSetMissing(ctx context.Context, id string, missing bool) error
	// ArtifactUsage sums the bytes the store holds: committed records plus the
	// staged records of attempts that are still running or finalizing.
	ArtifactUsage(ctx context.Context, project string) (Usage, error)
	// ArtifactPrune deletes (or, with DryRun, lists) the records spec allows.
	ArtifactPrune(ctx context.Context, spec PruneSpec) ([]Record, error)
	// ArtifactBlobRefs lists every blob some record still references.
	ArtifactBlobRefs(ctx context.Context) ([]BlobRef, error)
}
