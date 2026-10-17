package legacy

// isEnvVarName returns true if s looks like an environment variable name (UPPER_CASE).
func isEnvVarName(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}
