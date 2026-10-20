package adhoc

// Purpose: the argv of an ad-hoc job as it is recorded. The real argv runs
// (it is exec'd as given); a redacted copy is the only form stored or shown.
// Inputs:  the argv and the process-wide redaction registry.
// Outputs: Recorded (a redacted, bounded one-line form) and Register (values
// that look like secrets join the registry so job output is masked too).
// Constraints: token shapes (ghp_, sk_live_, AKIA, JWT...) are masked by the
// P7-TRUST-02 redactor with no help; a value that follows a secret-looking name
// (`--token=V`, `PASSWORD=V`, `--api-key V`) is registered, then masked. Nothing
// is guessed for plain words; known credential flags and structured payloads
// are also recognized.

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// maxRecorded bounds the recorded command line.
const maxRecorded = 240

var secretName = regexp.MustCompile(`(?i)(token|secret|passw(or)?d|passwd|credential|api[-_]?key|private[-_]?key|access[-_]?key|auth)`)

// secretValues returns the argv values that follow a secret-looking name.
func secretValues(argv []string) [][]byte {
	var out [][]byte
	for i, a := range argv {
		if i > 0 {
			prev := argv[i-1]
			// Register the complete argument as well as its sensitive component:
			// short values must still be hidden in Recorded by position.
			switch {
			case strings.HasPrefix(a, "-p") && len(a) > 2 && !strings.HasPrefix(a, "--") && (argv[0] == "mysql" || strings.HasSuffix(argv[0], "/mysql")):
				out = append(out, []byte(a), []byte(a[2:]))
			case prev == "-u" || prev == "--user" || strings.HasPrefix(a, "--user="):
				out = append(out, []byte(a))
				credentials := strings.TrimPrefix(a, "--user=")
				if _, password, ok := strings.Cut(credentials, ":"); ok && password != "" {
					out = append(out, []byte(password))
				}
			case (prev == "-H" || prev == "--header") && secretName.MatchString(strings.SplitN(a, ":", 2)[0]):
				out = append(out, []byte(a))
				if _, v, ok := strings.Cut(a, ":"); ok && strings.TrimSpace(v) != "" {
					out = append(out, []byte(strings.TrimSpace(v)))
				}
			case strings.Contains(a, "{") && secretName.MatchString(a):
				out = append(out, []byte(a))
				out = append(out, jsonSecrets(a)...)
			}
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !secretName.MatchString(name) {
			continue
		}
		switch {
		case hasValue && value != "":
			out = append(out, []byte(value))
		case !hasValue && strings.HasPrefix(a, "-") && i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "-"):
			out = append(out, []byte(argv[i+1]))
		}
	}
	return out
}

// Register adds the secret-looking values of argv to r and returns how many
// were too short for the registry (redact.MinValueLen): those are masked in
// Recorded by position but cannot be masked in job output without masking
// every short word, so the handler tells the operator.
func Register(r *redact.Redactor, argv []string) (tooShort int) {
	for _, v := range secretValues(argv) {
		if err := r.Register(v); err != nil {
			tooShort++
		}
	}
	return tooShort
}

// Recorded is the redacted one-line form of argv.
func Recorded(r *redact.Redactor, argv []string) string {
	hidden := map[string]bool{}
	for _, v := range secretValues(argv) {
		hidden[string(v)] = true
	}
	parts := make([]string, len(argv))
	for i, a := range argv {
		if name, value, ok := strings.Cut(a, "="); ok && hidden[value] {
			a = name + "=[redacted]"
		} else if hidden[a] {
			a = "[redacted]"
		}
		f := r.NewFilter()
		parts[i] = string(append(f.Redact([]byte(a)), f.Flush()...))
	}
	line := strings.Join(parts, " ")
	if len(line) > maxRecorded {
		cut := maxRecorded
		for cut > 0 && !utf8.RuneStart(line[cut]) { // never cut inside a rune
			cut--
		}
		line = line[:cut] + "…"
	}
	return line
}

// jsonSecrets registers string values below sensitive JSON keys for output masking.
func jsonSecrets(arg string) [][]byte {
	var value any
	if json.Unmarshal([]byte(arg), &value) != nil {
		return nil
	}
	var out [][]byte
	var walk func(any, bool)
	walk = func(v any, sensitive bool) {
		switch x := v.(type) {
		case string:
			if sensitive && x != "" {
				out = append(out, []byte(x))
			}
		case map[string]any:
			for key, child := range x {
				walk(child, sensitive || secretName.MatchString(key))
			}
		case []any:
			for _, child := range x {
				walk(child, sensitive)
			}
		}
	}
	walk(value, false)
	return out
}
