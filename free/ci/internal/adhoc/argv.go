package adhoc

// Purpose: the argv of an ad-hoc job as it is recorded. The real argv runs
// (it is exec'd as given); a redacted copy is the only form stored or shown.
// Inputs:  the argv and the process-wide redaction registry.
// Outputs: Recorded (a redacted, bounded one-line form) and Register (values
//          that look like secrets join the registry so job output is masked too).
// Constraints: token shapes (ghp_, sk_live_, AKIA, JWT...) are masked by the
//          P7-TRUST-02 redactor with no help; a value that follows a secret-looking name
//          (`--token=V`, `PASSWORD=V`, `--api-key V`) is registered, then masked. Well-known
//          positional forms (`-pVALUE`, `-u user:pass`, `-H X-Key: val`) are also caught.
//          Nothing else is guessed: an argument that is only a word stays visible.

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// maxRecorded bounds the recorded command line.
const maxRecorded = 240

var secretName = regexp.MustCompile(`(?i)(token|secret|passw(or)?d|passwd|credential|api[-_]?key|private[-_]?key|access[-_]?key|auth)`)

// mentionsMySQLClient reports whether argv names a MySQL-family client
// (mysql, mysqldump, mariadb, mariadb-dump...), directly or behind a wrapper
// such as `docker exec db mysql`.
func mentionsMySQLClient(argv []string) bool {
	for _, a := range argv {
		base := a
		if i := strings.LastIndexAny(base, `/\`); i >= 0 {
			base = base[i+1:]
		}
		if strings.HasPrefix(base, "mysql") || strings.HasPrefix(base, "mariadb") {
			return true
		}
	}
	return false
}

// isMySQLPassword reports whether a is the attached-password form `-pVALUE`.
func isMySQLPassword(a string) bool { return strings.HasPrefix(a, "-p") && len(a) > 2 }

// wellKnownSecrets returns values carried by well-known positional forms that
// do not use a secret-looking flag name: mysql-style `-pVALUE`, curl-style
// `-u user:pass` and `-H "X-Key: value"` headers whose name looks secret-like.
func wellKnownSecrets(argv []string) [][]byte {
	var out [][]byte
	mysql := mentionsMySQLClient(argv)
	for i, a := range argv {
		if len(a) < 2 || a[0] != '-' {
			continue
		}
		switch {
		case mysql && isMySQLPassword(a):
			// mysql/mariadb: -pS3cret (no space). Only when a MySQL-family client is in
			// the command, so `go test -parallel 4`, `-pkgdir`, `-print` stay visible.
			out = append(out, []byte(a[2:]))
		case a == "-u" && i+1 < len(argv) && strings.Contains(argv[i+1], ":"):
			// curl -u user:pass
			out = append(out, []byte(argv[i+1]))
		case a == "-H" && i+1 < len(argv):
			// -H "X-Api-Key: value" — mask the value when the header name is secret-looking.
			hdr := argv[i+1]
			if name, value, ok := strings.Cut(hdr, ":"); ok && secretName.MatchString(name) {
				out = append(out, []byte(strings.TrimSpace(value)))
			}
		}
	}
	return out
}

// secretValues returns the argv values that follow a secret-looking name plus
// any well-known positional forms.
func secretValues(argv []string) [][]byte {
	var out [][]byte
	for i, a := range argv {
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !secretName.MatchString(name) {
			continue
		}
		switch {
		case hasValue && value != "":
			out = append(out, []byte(value))
		case !hasValue && strings.HasPrefix(a, "-") && i+1 < len(argv):
			// A secret value may itself begin with a dash.
			out = append(out, []byte(argv[i+1]))
		}
	}
	out = append(out, wellKnownSecrets(argv)...)
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
	mysql := mentionsMySQLClient(argv)
	parts := make([]string, len(argv))
	for i, a := range argv {
		if mysql && isMySQLPassword(a) { // masked by position: a value too short for the registry is hidden too
			parts[i] = "-p[redacted]"
			continue
		}
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
