package sched

import (
	"math"
	"math/bits"
)

// Add saturates a signed sum and reports overflow.
func Add(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64, true
	}
	if b < 0 && a < math.MinInt64-b {
		return math.MinInt64, true
	}
	return a + b, false
}

// Mul saturates a signed product and reports overflow.
func Mul(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, false
	}
	if a == -1 && b == math.MinInt64 || b == -1 && a == math.MinInt64 {
		return math.MaxInt64, true
	}
	c := a * b
	if c/b != a {
		if (a < 0) != (b < 0) {
			return math.MinInt64, true
		}
		return math.MaxInt64, true
	}
	return c, false
}

// MulDivUp returns ceil(a*b/d) for a, b >= 0 and d > 0 through a 128-bit product, so a
// large a*b that d brings back into range is exact. A quotient past int64, or an
// argument out of range, saturates and reports overflow.
func MulDivUp(a, b, d int64) (int64, bool) {
	if a < 0 || b < 0 || d <= 0 {
		return math.MaxInt64, true
	}
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	lo, carry := bits.Add64(lo, uint64(d-1), 0)
	hi += carry
	if hi >= uint64(d) {
		return math.MaxInt64, true
	}
	q, _ := bits.Div64(hi, lo, uint64(d))
	if q > math.MaxInt64 {
		return math.MaxInt64, true
	}
	return int64(q), false
}
