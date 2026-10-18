// Package env handles allowlist-only env var injection and secret redaction.
package env

import (
	"regexp"
	"strings"
)

// secretPatterns matches env variable values that look like secrets.
// These are redacted in log output before storage.
var secretPatterns = regexp.MustCompile(
	`(?i)((_KEY|_SECRET|_TOKEN|_PASSWORD|_PASS|_PWD)\s*=\s*)(\S+)`,
)

// BuildEnv constructs the environment map for a Deno isolate invocation.
// Only variables explicitly listed in allowedKeys are included.
// allEnv is the full function env map (from np_edge_function_env).
func BuildEnv(allEnv map[string]string, allowedKeys []string) map[string]string {
	if len(allowedKeys) == 0 {
		return map[string]string{}
	}
	allowed := make(map[string]bool, len(allowedKeys))
	for _, k := range allowedKeys {
		allowed[strings.ToUpper(k)] = true
	}
	result := make(map[string]string)
	for k, v := range allEnv {
		if allowed[strings.ToUpper(k)] {
			result[k] = v
		}
	}
	return result
}

// RedactSecrets replaces secret values in a log message with [REDACTED].
// Applied server-side before writing to np_function_logs.
func RedactSecrets(message string) string {
	return secretPatterns.ReplaceAllStringFunc(message, func(match string) string {
		// Find the separator and keep everything up to and including the '='
		idx := strings.Index(match, "=")
		if idx < 0 {
			return match
		}
		return match[:idx+1] + "[REDACTED]"
	})
}
