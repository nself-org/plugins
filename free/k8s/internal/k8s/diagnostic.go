// Purpose: make captured Helm diagnostics safe for terminal and JSON output.
// Inputs: Helm stderr or recovery advice. Outputs: sanitized, bounded UTF-8 text.
// Constraints: omit URL credentials and paths, credential assignments, terminal
// controls and the configured licence key; retain at most 512 tail bytes plus ... .
package k8s

import (
	"net/url"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	helmEscape     = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))`)
	helmURL        = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^\s<>"']+`)
	helmCredential = regexp.MustCompile(`(?i)((?:password|passwd|token|secret|api[_-]?key|authorization)\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
	helmBearer     = regexp.MustCompile(`(?i)\bBearer\s+[^\s,;]+`)
)

// SafeHelmDiagnostic sanitizes before truncating so long credentials cannot
// lose their identifying prefix and expose the retained tail.
func SafeHelmDiagnostic(text string) string {
	text = helmEscape.ReplaceAllString(text, "")
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return ' '
		}
		return r
	}, text)
	text = helmURL.ReplaceAllStringFunc(text, func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return "[redacted URL]"
		}
		return u.Scheme + "://" + u.Host
	})
	text = helmBearer.ReplaceAllString(text, "Bearer [redacted]")
	text = helmCredential.ReplaceAllString(text, "${1}[redacted]")
	if key := os.Getenv(LicenseEnv); key != "" {
		text = strings.ReplaceAll(text, key, "[redacted]")
	}
	text = strings.TrimSpace(text)
	if len(text) > maxStderrTail {
		text = text[len(text)-maxStderrTail:]
		for len(text) > 0 && !utf8.RuneStart(text[0]) {
			text = text[1:]
		}
		text = "..." + text
	}
	return text
}
