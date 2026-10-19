package history

// Purpose: the retention policy of run history and stored logs, and where a
// user changes it.
// Inputs:  <state dir>/retention.json, for example
//          {"keep_runs": 100, "max_age_days": 14, "max_log_bytes": 1073741824}.
// Outputs: a Policy with every limit set.
// Constraints: defaults are the last 200 runs, 30 days and 2 GiB of stored
// logs; a field that is absent or not positive keeps its default; an
// unreadable or malformed file keeps all defaults and is reported, never a
// reason to prune more.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/logs"
)

// Retention defaults.
const (
	DefaultKeepRuns    = 200
	DefaultMaxAgeDays  = 30
	DefaultMaxLogBytes = int64(2) << 30
)

// Policy is how much history is kept.
type Policy struct {
	KeepRuns    int   `json:"keep_runs"`
	MaxAgeDays  int   `json:"max_age_days"`
	MaxLogBytes int64 `json:"max_log_bytes"`
}

// Default is the policy when nothing is configured.
func Default() Policy {
	return Policy{KeepRuns: DefaultKeepRuns, MaxAgeDays: DefaultMaxAgeDays, MaxLogBytes: DefaultMaxLogBytes}
}

// MaxAge is the age limit as a duration.
func (p Policy) MaxAge() time.Duration { return time.Duration(p.MaxAgeDays) * 24 * time.Hour }

// ConfigPath is the user's retention file for a state directory.
func ConfigPath(home string) string { return filepath.Join(logs.Home(home), "retention.json") }

// LoadPolicy reads the user's retention file over the defaults. The returned
// error describes a file that exists but could not be used; the policy is
// still the defaults in that case.
func LoadPolicy(home string) (Policy, error) {
	p := Default()
	b, err := os.ReadFile(ConfigPath(home))
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	var in Policy
	if err = json.Unmarshal(b, &in); err != nil {
		return p, err
	}
	if in.KeepRuns > 0 {
		p.KeepRuns = in.KeepRuns
	}
	if in.MaxAgeDays > 0 {
		p.MaxAgeDays = in.MaxAgeDays
	}
	if in.MaxLogBytes > 0 {
		p.MaxLogBytes = in.MaxLogBytes
	}
	return p, nil
}
