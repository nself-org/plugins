package adhoc

// Purpose: the argv of an ad-hoc job as it is recorded. The real argv runs
// (it is exec'd as given); a redacted copy is the only form stored or shown.
// Inputs:  the argv and the process-wide redaction registry.
// Outputs: Recorded (a redacted, bounded one-line form) and Register (values
// that look like secrets join the registry so job output is masked too).
// Constraints: token shapes (ghp_, sk_live_, AKIA, JWT...) are masked by the
// P7-TRUST-02 redactor with no help; a value that follows a secret-looking name
// (`--token=V`, `PASSWORD=V`, `--api-key V`) is registered, then masked. Nothing
// is guessed beyond that: an argument that is only a word stays visible,
// except the well-known positional forms (mysql -p<pw>, curl -u user:pass,
// secret headers, JSON and form bodies, URL userinfo), found in argv_forms.go.

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// maxRecorded bounds the recorded command line.
const maxRecorded = 240

var secretName = regexp.MustCompile(`(?i)(token|secret|passw(or)?d|passwd|credential|api[-_]?key|private[-_]?key|access[-_]?key|auth)`)

// secretSpans returns the secret parts of argv: values that follow a
// secret-looking name, then the positional forms of argv_forms.go.
func secretSpans(argv []string) []span {
	var out []span
	for i, a := range argv {
		trimmed := strings.TrimLeft(a, "-")
		off := len(a) - len(trimmed)
		name, value, hasValue := strings.Cut(trimmed, "=")
		if !secretName.MatchString(name) {
			continue
		}
		switch {
		case hasValue && value != "":
			out = append(out, span{arg: i, from: off + len(name) + 1, to: len(a)})
		case !hasValue && strings.HasPrefix(a, "-") && i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "-"):
			out = append(out, span{arg: i + 1, to: len(argv[i+1])})
		}
	}
	return append(out, formSpans(argv)...)
}

// secretValues returns the secret values of argv.
func secretValues(argv []string) [][]byte {
	var out [][]byte
	for _, s := range secretSpans(argv) {
		for _, p := range pieces(argv[s.arg][s.from:s.to], s.parts) {
			out = append(out, []byte(p))
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
	marked := make([][]bool, len(argv)) // marked[i][b]: byte b of argv[i] is secret
	for _, sp := range secretSpans(argv) {
		hidden[argv[sp.arg][sp.from:sp.to]] = true
		if marked[sp.arg] == nil {
			marked[sp.arg] = make([]bool, len(argv[sp.arg]))
		}
		for b := sp.from; b < sp.to; b++ {
			marked[sp.arg][b] = true
		}
	}
	parts := make([]string, len(argv))
	for i, a := range argv {
		if hidden[a] {
			a = "[redacted]"
		} else if marked[i] != nil {
			a = maskMarked(a, marked[i])
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

// maskMarked replaces each run of marked bytes of a with one [redacted].
func maskMarked(a string, marked []bool) string {
	var b strings.Builder
	for i := 0; i < len(a); i++ {
		if !marked[i] {
			b.WriteByte(a[i])
		} else if i == 0 || !marked[i-1] {
			b.WriteString("[redacted]")
		}
	}
	return b.String()
}
