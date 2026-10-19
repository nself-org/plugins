package detach

// Purpose: the redacted tail of a failed child's stream (P7-CI-63): E648
// child_exit and no_queued_event show the last 2 KiB of what the child wrote.
// Inputs:  the stream file and the parent environment.
// Outputs: text safe to print.
// Constraints: token shapes and the values of secret-looking variables are
// masked by internal/redact before the text leaves this package.

import (
	"io"
	"os"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// TailBytes is how much of the child stream an error carries.
const TailBytes = 2048

// secretNames marks variables whose value is masked when it appears in the tail.
var secretNames = []string{"TOKEN", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL", "APIKEY", "API_KEY", "PRIVATE_KEY", "AUTH"}

func redactor(env []string) *redact.Redactor {
	r := redact.New()
	for _, kv := range env {
		name, val, ok := strings.Cut(kv, "=")
		if !ok || len(val) < redact.MinValueLen {
			continue
		}
		up := strings.ToUpper(name)
		for _, s := range secretNames {
			if strings.Contains(up, s) {
				_ = r.Register([]byte(val))
				break
			}
		}
	}
	return r
}

// RedactedTail returns the last TailBytes of the file with secrets masked.
func RedactedTail(path string, env []string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	start := st.Size() - TailBytes
	if start < 0 {
		start = 0
	}
	if _, err = f.Seek(start, io.SeekStart); err != nil {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(f, TailBytes))
	if err != nil {
		return ""
	}
	flt := redactor(env).NewFilter()
	out := append(flt.Redact(b), flt.Flush()...)
	return strings.TrimSpace(strings.ToValidUTF8(string(out), "�"))
}
