package internal

import (
	"strconv"
	"time"
)

func parseTimePtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// Try RFC3339Nano as well.
		t, err = time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return nil
		}
	}
	return &t
}

// parseFloat converts a string to float64, returning 0 on failure.
func parseFloat(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// nilIfEmpty returns nil if the string is empty, otherwise a pointer to it.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// joinNames concatenates non-nil name parts with a space.
func joinNames(parts ...*string) string {
	var result []string
	for _, p := range parts {
		if p != nil && *p != "" {
			result = append(result, *p)
		}
	}
	if len(result) == 0 {
		return ""
	}
	out := result[0]
	for i := 1; i < len(result); i++ {
		out += " " + result[i]
	}
	return out
}
