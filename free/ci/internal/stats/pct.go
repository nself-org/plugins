package stats

// Purpose: nearest-rank percentiles and permille rates from integers.
// Inputs:  integer-millisecond samples; a numerator and denominator.
// Outputs: *Pct (nil for no samples) and an integer permille.
// Constraints: no floating point; the input slice is not modified.

import "slices"

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

// permille returns num/den in thousandths, rounded half up; den must be positive.
func permille(num, den int64) int { return int((num*1000 + den/2) / den) }
