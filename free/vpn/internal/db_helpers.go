package internal

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func joinAnd(parts []string) string {
	if len(parts) == 0 {
		return "TRUE"
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += " AND " + p
	}
	return result
}

func coalesceStr(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

