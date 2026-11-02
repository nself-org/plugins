package stats

// Purpose: nearest-rank percentiles and permille rates from integers.
// Inputs:  integer-millisecond samples (a list, or distinct values with their counts); a numerator and denominator.
// Outputs: *Pct (nil for no samples) and an integer permille.
// Constraints: no floating point; the input slice is not modified.

import (
	"cmp"
	"slices"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// pct returns the nearest-rank P50 and P95 of v, or nil when v is empty.
func pct(v []int64) *Pct {
	if len(v) == 0 {
		return nil
	}
	s := slices.Clone(v)
	slices.Sort(s)
	rank := func(p int) int64 {
		r := (p*len(s) + 99) / 100 // ceil(p*n/100)
		return s[max(r, 1)-1]
	}
	return &Pct{P50: rank(50), P95: rank(95), N: len(s)}
}

// pctSamples is pct over distinct values with their counts: the same nearest-rank P50 and P95 as the list
// that repeats each value N times, without building that list.
func pctSamples(v []store.StatSample) *Pct {
	var n int64
	for _, x := range v {
		n += x.N
	}
	if n == 0 {
		return nil
	}
	s := slices.Clone(v)
	slices.SortFunc(s, func(a, b store.StatSample) int { return cmp.Compare(a.Ms, b.Ms) })
	rank := func(p int64) int64 {
		r, seen := max((p*n+99)/100, 1), int64(0)
		for _, x := range s {
			if seen += x.N; seen >= r {
				return x.Ms
			}
		}
		return s[len(s)-1].Ms
	}
	return &Pct{P50: rank(50), P95: rank(95), N: int(n)}
}

// permille returns num/den in thousandths, rounded half up; den must be positive.
func permille(num, den int64) int { return int((num*1000 + den/2) / den) }
