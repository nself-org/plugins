package delta

import "strings"

// Name returns the package name in upper case.
func Name() string { return strings.ToUpper("delta") }

// Sum adds the values.
func Sum(values ...int) int {
	total := 0
	for _, v := range values {
		total += v
	}
	return total
}

// Repeat joins n copies of s with a comma.
func Repeat(s string, n int) string {
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, s)
	}
	return strings.Join(parts, ",")
}
