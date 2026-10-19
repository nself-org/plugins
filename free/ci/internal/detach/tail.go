package detach

// Purpose: the redacted tail of a failed child's stream (P7-CI-63): E648
// child_exit and no_queued_event show the last 2 KiB of what the child wrote.
// Inputs:  the stream file and the parent environment.
// Outputs: text safe to print.
// Constraints: token shapes and the values of secret-looking variables are
// masked by internal/redact before the text leaves this package.

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// TailBytes is how much of the child stream an error carries.
const TailBytes = 2048

// scanBytes is how much of the end of the file is redacted before the tail is
// cut: a secret that straddles the TailBytes boundary must be masked whole.
const scanBytes = 64 << 10

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

// RedactedTail returns the last TailBytes of the file with secrets masked. The
// last scanBytes are redacted first and the tail is cut from the masked text.
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
	start := max(st.Size()-scanBytes, 0)
	if _, err = f.Seek(start, io.SeekStart); err != nil {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(f, scanBytes))
	if err != nil {
		return ""
	}
	if start > 0 { // begin at a line start: a cut line could hide a secret's head
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	flt := redactor(env).NewFilter()
	out := append(flt.Redact(b), flt.Flush()...)
	if len(out) > TailBytes {
		out = out[len(out)-TailBytes:]
	}
	return strings.TrimSpace(strings.ToValidUTF8(string(out), "\uFFFD"))
}
