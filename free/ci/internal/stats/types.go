package stats

// Purpose: contract:ci.stats v1 (P7-SCHED-20, Epic SCHED D23): the document `nself ci runs stats` prints and
// GET /v1/stats serves. Experimental until P8; the golden fixtures in testdata are its definition.
// Inputs:  none (types only).
// Outputs: Doc and its blocks.
// Constraints: an absent source is null with a reason in Doc.Reasons (dotted path -> reason), never 0; rates are
//          integer permille; durations are integer milliseconds; maps marshal in key order, so the output is stable.

// Contract names the document; Version is its schema version.
const (
	Contract = "ci.stats"
	Version  = 1
)

// Reasons a value is null.
const (
	ReasonNoSamples     = "no_samples"
	ReasonNoRows        = "no_rows"
	ReasonNotInstalled  = "not_installed"
	ReasonNoLookups     = "no_lookups"
	ReasonNonePending   = "none_pending"
	ReasonSlotsUnknown  = "slots_unknown"
	ReasonScoped        = "scoped_to_projects"
	ReasonStoreUnstated = "store_unreadable"
)

// Pct is a nearest-rank percentile pair over N integer-millisecond samples.
type Pct struct {
	P50 int64 `json:"p50"`
	P95 int64 `json:"p95"`
	N   int   `json:"n"`
}

// Band is the queue state of one fair-queue band.
type Band struct {
	Depth  int  `json:"depth"`
	WaitMs *Pct `json:"wait_ms"`
}

// Runner is one runner's use in the window.
type Runner struct {
	Attempts            int    `json:"attempts"`
	BusyMs              int64  `json:"busy_ms"`
	Slots               *int   `json:"slots"`
	AvailableMs         *int64 `json:"available_ms"`
	UtilisationPermille *int   `json:"utilisation_permille"`
}

// Attempts holds the outcome rates over finished attempts.
type Attempts struct {
	Finished           int `json:"finished"`
	PassPermille       int `json:"pass_permille"`
	FailPermille       int `json:"fail_permille"`
	FlakyPermille      int `json:"flaky_permille"`
	InfraRetryPermille int `json:"infra_retry_permille"`
}

// Cache is the cache use of the window.
type Cache struct {
	Hits          int64 `json:"hits"`
	Misses        int64 `json:"misses"`
	HitPermille   *int  `json:"hit_permille"`
	BytesRestored int64 `json:"bytes_restored"`
	BytesSaved    int64 `json:"bytes_saved"`
}

// Storage is what the CAS and the artifact store hold now.
type Storage struct {
	CASBytes      *int64 `json:"cas_bytes"`
	ArtifactBytes *int64 `json:"artifact_bytes"`
}

// Spend is the cost recorded at one confidence.
type Spend struct {
	Attempts int64 `json:"attempts"`
	Micro    int64 `json:"micro"`
}

// Leases counts leases by condition; a lease is in exactly one.
type Leases struct {
	Active     int `json:"active"`
	Stale      int `json:"stale"`
	SelfFenced int `json:"self_fenced"`
}

// Nodes counts nodes by state.
type Nodes struct {
	ByState        map[string]int `json:"by_state"`
	OfflineOver24h int            `json:"offline_over_24h"`
}

// Outbox is the status-report backlog.
type Outbox struct {
	Pending     int64  `json:"pending"`
	OldestAgeMs *int64 `json:"oldest_age_ms"`
}

// StoreInfo is the size of the run store and the free space of its volume.
type StoreInfo struct {
	SizeBytes int64  `json:"size_bytes"`
	FreeBytes *int64 `json:"free_bytes"`
}

// Doc is contract:ci.stats v1.
type Doc struct {
	Contract     string            `json:"contract"`
	Version      int               `json:"version"`
	GeneratedAt  string            `json:"generated_at"`
	SinceMs      int64             `json:"since_ms"`
	Project      *string           `json:"project"`
	Queue        map[string]Band   `json:"queue"`
	Runners      map[string]Runner `json:"runners"`
	Attempts     *Attempts         `json:"attempts"`
	Jobs         map[string]Pct    `json:"jobs"`
	Cache        *Cache            `json:"cache"`
	Storage      *Storage          `json:"storage"`
	Provisioning *Pct              `json:"provisioning_ms"`
	Spend        map[string]Spend  `json:"spend"`
	Leases       *Leases           `json:"leases"`
	Nodes        *Nodes            `json:"nodes"`
	Outbox       *Outbox           `json:"outbox"`
	Store        *StoreInfo        `json:"store"`
	Reasons      map[string]string `json:"reasons"`
}
