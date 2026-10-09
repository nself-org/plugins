package sched

import "math"

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
