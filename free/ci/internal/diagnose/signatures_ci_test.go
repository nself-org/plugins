package diagnose

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// TestAutoSignatures pins the P7-AUTO-16 rows: an unreachable provider and the
// extra disk-full spellings are retryable infrastructure; a refused local
// connection and a failing test that mentions them are code.
func TestAutoSignatures(t *testing.T) {
	for _, c := range []struct {
		name string
		in   Input
		want model.FailureClass
		sig  string
	}{
		{"npm registry timeout", Input{ExecClass: "code", ExitCode: 1, Tail: "npm ERR! code ETIMEDOUT\nnpm ERR! network This is a problem related to network connectivity.\n"}, "infra", SigNetwork},
		{"connection reset by peer", Input{ExecClass: "code", ExitCode: 56, Tail: "curl: (56) Recv failure: Connection reset by peer\n"}, "infra", SigNetwork},
		{"tls handshake timeout", Input{ExecClass: "code", ExitCode: 1, Tail: "Get \"https://registry.example/v2/\": net/http: TLS handshake timeout\n"}, "infra", SigNetwork},
		{"executor error carries it", Input{ExecClass: "infra", ExitCode: -1, Err: "read tcp 10.0.0.2:5->10.0.0.9:443: i/o timeout"}, "infra", SigNetwork},
		{"pnpm fetch", Input{ExecClass: "code", ExitCode: 1, Tail: " ERR_PNPM_META_FETCH_FAIL  GET https://registry.example/x: ECONNRESET\n"}, "infra", SigNetwork},
		{"enospc", Input{ExecClass: "code", ExitCode: 1, Tail: "npm ERR! code ENOSPC\n"}, "infra", SigDisk},
		{"windows disk full", Input{ExecClass: "code", ExitCode: 1, Tail: "There is not enough space on the disk.\n"}, "infra", SigDisk},
		{"refused on localhost is the project's setup", Input{ExecClass: "code", ExitCode: 1, Tail: "dial tcp 127.0.0.1:5432: connect: connection refused\n"}, "code", SigCode},
		{"a failing test mentioning a timeout is code", Input{ExecClass: "code", ExitCode: 1, Tail: "--- FAIL: TestRetry (ETIMEDOUT expected)\nFAIL\n"}, "code", SigCode},
	} {
		d := Classify(c.in)
		if d.Class != c.want || d.Signature != c.sig || (c.want == "infra") != d.Retryable || d.Remediation == "" {
			t.Errorf("%s: got %+v, want %s %s", c.name, d, c.want, c.sig)
		}
	}
}

// TestFirstRowStaysOOM guards the index Classify uses for a container resource limit.
func TestFirstRowStaysOOM(t *testing.T) {
	if signatures[0].id != SigOOM {
		t.Fatalf("signatures[0] is %s", signatures[0].id)
	}
}
