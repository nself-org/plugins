package entries

// Purpose: usage rows (attempt, spec, key, result, bytes, ms) for the cache
// views and the benchmarks.
// Inputs: Use. Outputs: stored rows by attempt.
// Constraints: only enumerated, validated fields are stored: no env value, no
// secret value and no path ever enters a usage row.

import "context"

var useResults = map[string]bool{"hit": true, "miss": true, "disabled": true, "readonly": true, "denied": true, "refused": true}

// RecordUse appends one usage row; the backend stamps the time.
func (s *Store) RecordUse(ctx context.Context, u Use) error {
	switch {
	case !attemptID.MatchString(u.Attempt):
		return bad("usage attempt id")
	case !specID.MatchString(u.Spec):
		return bad("usage spec id")
	case u.Key != "" && !hexKey.MatchString(u.Key):
		return bad("usage key")
	case !useResults[u.Result]:
		return bad("usage result %q", u.Result)
	case u.Bytes < 0 || u.MS < 0:
		return bad("negative usage figure")
	}
	return s.be.CacheRecordUse(ctx, u)
}

// Usage lists the usage rows of one attempt, oldest first.
func (s *Store) Usage(ctx context.Context, attempt string) ([]Use, error) {
	return s.be.CacheUses(ctx, attempt)
}
