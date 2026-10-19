package entries

// Purpose: the data types shared by the entries Store and its backend, and the
// Backend interface *store.Store implements (SQL lives only in internal/store).
// Inputs: none. Outputs: Manifest, Blob, Writer, Facts, CommitRequest, PruneStep,
// Pruned, Use, Backend.
// Constraints: pure; no os/exec, database/sql or net. A manifest carries digests,
// sizes and relative paths only: never an env value, secret value or token.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

// Blob is one cas blob of an entry: a clean relative path, its digest and size.
type Blob struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Writer records who committed the entry. The backend fills it from the facts it
// stored at restore time; a caller-supplied value is ignored on commit.
type Writer struct {
	AttemptID       string `json:"attempt_id"`
	Epoch           int64  `json:"epoch"`
	SourceTrust     string `json:"source_trust"`
	SecretsReceived bool   `json:"secrets_received"`
	PrivateDeps     bool   `json:"private_deps"`
	ScannedClean    bool   `json:"scanned_clean"`
}

// Manifest is an entry as stored. Namespace is authz.Namespace.Canonical().
type Manifest struct {
	Namespace  []byte `json:"namespace"`
	Key        string `json:"key"`
	Spec       string `json:"spec"`
	Blobs      []Blob `json:"blobs"`
	Writer     Writer `json:"writer"`
	CreatedAt  int64  `json:"created_at"`
	LastUsedAt int64  `json:"last_used_at"`
}

// Bytes is the total size of the entry's blobs.
func (m Manifest) Bytes() (n int64) {
	for _, b := range m.Blobs {
		n += b.Size
	}
	return n
}

// Facts are the job facts the coordinator records when the attempt restores.
type Facts struct {
	Project, Trust, Protection, HeadRepo, PR string
	SecretsDeclared, PrivateDeps             bool
}

// CommitRequest is one cache save. Claimed is only compared with the namespace
// the backend recomputes from the stored facts. Check, when set, runs inside the
// commit transaction after every authorization check and before the insert, so
// nothing it reveals (blob presence, free space) reaches an unauthorised caller;
// a *Refused it returns is recorded as a refusal, any other error aborts.
type CommitRequest struct {
	Attempt       string
	Epoch         int64
	Claimed       authz.Namespace
	Manifest      Manifest
	MaxEntryBytes int64
	Check         func() error
}

// PruneStep bounds one backend prune transaction (see Backend.CachePrune).
type PruneStep struct {
	MaxAge, Retention time.Duration
	MaxBytes          int64
	FreeBytes         int64
	Limit             int
}

// Pruned is what one prune step removed; More means Limit stopped it early.
type Pruned struct {
	Entries int
	Bytes   int64
	More    bool
}

// Use is one restore or save outcome. Result is hit, miss, disabled, readonly,
// denied or refused.
type Use struct {
	Attempt, Spec, Key, Result string
	Bytes, MS, TS              int64
}

// Backend is the persistence entries needs; *store.Store implements it.
type Backend interface {
	CacheRecordPlan(ctx context.Context, attempt string, epoch int64, f Facts, keys []string) error
	CacheRecordResult(ctx context.Context, attempt string, epoch int64, passed bool) error
	CacheCommit(ctx context.Context, c CommitRequest) error
	// CacheEntries lists the entries stored under key (one per namespace), or all
	// entries when key is "", least recently used first, each from one snapshot.
	CacheEntries(ctx context.Context, key string) ([]Manifest, error)
	CacheTouch(ctx context.Context, ns []byte, key string) error
	CacheDrop(ctx context.Context, m Manifest, reason string) error
	CachePrune(ctx context.Context, p PruneStep) (Pruned, error)
	CacheRecordUse(ctx context.Context, u Use) error
	CacheUses(ctx context.Context, attempt string) ([]Use, error)
	AppendAudit(ctx context.Context, actor, action, target, detail string) error
}
