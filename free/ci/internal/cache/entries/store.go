package entries

// Purpose: the cache entries Store (cap:ci.cache): the high-level API over a
// Backend (SQL, in internal/store) and the cas blob store.
// Inputs: Backend, *cas.Store, Options. Outputs: Store with Key (key.go), Lookup,
// Commit, RecordPlan, RecordResult, RecordUse, Prune, Verify, Usage.
// Constraints: no os/exec, database/sql or net. Caps come from
// NSELF_CI_CACHE_MAX_BYTES (default 10 GiB) and NSELF_CI_CACHE_MAX_AGE (default
// 14 d); an unparsable value keeps the default and prints one warning. The
// store free-space floor is NSELF_CI_STORE_MIN_FREE (default the larger of 5 GiB
// and 10 % of the volume). No env or secret value is ever persisted here.

import (
	"io"
	"os"
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
)

const (
	defaultMaxBytes = int64(10) << 30
	defaultMaxAge   = 336 * time.Hour
	defaultMinFree  = int64(5) << 30
	// BlobMaxAge is how old a blob may be when its entry commits. It is half of
	// cas's one hour sweep grace, so a blob that Commit accepts cannot be swept by a
	// concurrent prune in this or any other process, whatever reference set that
	// prune scanned. cas.Put replaces an existing blob, so re-Putting refreshes it.
	BlobMaxAge = 30 * time.Minute
	// UsageRetention is how long usage rows and finished-attempt facts live.
	UsageRetention = 30 * 24 * time.Hour
)

// Options tune a Store. The zero value reads the environment, uses the wall
// clock and stat(2) on Home (default NSELF_CI_HOME), and treats every project as
// having private dependencies (ro-snapshot serving off until Project says
// otherwise). With neither Home nor Free, Commit and Prune fail rather than
// silently skip the free-space floor, unless DisableFloor is set.
type Options struct {
	DisableFloor bool
	Home         string                                // NSELF_CI_HOME, for free-space checks
	Getenv       func(string) string                   // default os.Getenv
	Warn         io.Writer                             // default os.Stderr
	Clock        func() time.Time                      // default time.Now
	Project      func(string) authz.ProjectFacts       // project-level snapshot switch
	Free         func() (free, total int64, err error) // default statfs of Home
}

// Store is safe for concurrent use.
type Store struct {
	be       Backend
	blobs    *cas.Store
	opt      Options
	maxBytes int64
	maxAge   time.Duration
	minFree  int64 // <0: derive from the volume
	now      func() time.Time
	open     func(cas.Realm, string) (io.ReadCloser, error) // cas.Open; replaced in tests
	stat     func(cas.Realm, string) (os.FileInfo, error)   // cas.Stat; replaced in tests
}

// New builds a Store and reads the environment caps once.
func New(be Backend, blobs *cas.Store, opt Options) *Store {
	if opt.Getenv == nil {
		opt.Getenv = os.Getenv
	}
	if opt.Home == "" {
		opt.Home = opt.Getenv("NSELF_CI_HOME")
	}
	if opt.Warn == nil {
		opt.Warn = os.Stderr
	}
	s := &Store{be: be, blobs: blobs, opt: opt, now: opt.Clock, maxBytes: defaultMaxBytes, maxAge: defaultMaxAge, minFree: -1}
	if s.now == nil {
		s.now = time.Now
	}
	if blobs != nil {
		s.open, s.stat = blobs.Open, blobs.Stat
	}
	warn := func(name, v string) {
		_, _ = io.WriteString(opt.Warn, "nself-ci: ignoring unparsable "+name+"="+strconv.Quote(v)+"\n")
	}
	if v := opt.Getenv("NSELF_CI_CACHE_MAX_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			s.maxBytes = n
		} else {
			warn("NSELF_CI_CACHE_MAX_BYTES", v)
		}
	}
	if v := opt.Getenv("NSELF_CI_CACHE_MAX_AGE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			s.maxAge = d
		} else {
			warn("NSELF_CI_CACHE_MAX_AGE", v)
		}
	}
	if v := opt.Getenv("NSELF_CI_STORE_MIN_FREE"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			s.minFree = n
		} else {
			warn("NSELF_CI_STORE_MIN_FREE", v)
		}
	}
	return s
}

// Caps reports the effective byte and age caps.
func (s *Store) Caps() (int64, time.Duration) { return s.maxBytes, s.maxAge }
