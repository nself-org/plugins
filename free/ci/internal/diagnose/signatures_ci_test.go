package diagnose

import (
	"strings"
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

func TestClassifyInstall(t *testing.T) {
	for name, c := range map[string]struct {
		log  string
		want model.FailureClass
		sig  string
	}{
		"dns far above the footer":            {"npm error network getaddrinfo ENOTFOUND registry.npmjs.org\n" + footer, "infra", SigDNS},
		"socket hang up":                      {"npm error network socket hang up\n" + footer, "infra", SigNetwork},
		"registry refuses":                    {"npm error code ECONNREFUSED\nnpm error FetchError: request to https://registry.npmjs.org/x failed, reason: connect ECONNREFUSED 10.1.2.3:443\n" + footer, "infra", SigNetwork},
		"refused without a registry":          {"npm error code ECONNREFUSED\n" + footer, "code", SigCode},
		"retry warnings are dropped":          {"npm warn ECONNRESET retrying\nnpm warn ECONNRESET retrying\nnpm error code ENOSPC\n" + footer, "infra", SigDisk},
		"warned transient then a build error": {"npm warn ECONNRESET retrying\n> app@1.0.0 postinstall\nsrc/a.ts(1,1): error TS2322: bad\nnpm error command failed\n", "code", SigCode},
		"pnpm retry then gyp":                 {"WARN  GET https://r/x error (ETIMEDOUT). Will retry in 10 seconds. 2 retries left.\ngyp ERR! build error\n ELIFECYCLE  Command failed with exit code 1.\n", "code", SigCode},
		"postinstall hits a local database":   {"> app@1.0.0 postinstall\n> node scripts/migrate.js\nError: connect ECONNREFUSED 127.0.0.1:5432\nnpm error command failed\n", "code", SigCode},
		"node assertion":                      {"AssertionError [ERR_ASSERTION]: expected ECONNREFUSED to be handled\n" + footer, "code", SigCode},
		"sync guard beyond the window":        {"ERR_PNPM_OUTDATED_LOCKFILE  not up to date\n" + strings.Repeat("detail line\n", 70) + "npm error code ECONNRESET\n", "code", SigCode},
		"enospc":                              {"npm error code ENOSPC\n" + footer, "infra", SigDisk},
		"sync beats a transient":              {"npm warn ECONNRESET retrying\nERR_PNPM_OUTDATED_LOCKFILE  not up to date\n", "code", SigCode},
		"yarn berry sync":                     {"YN0028: The lockfile would have been modified by this install\n", "code", SigCode},
		"404 is the project's":                {" ERR_PNPM_FETCH_404  GET https://r/x: Not Found - 404\n", "code", SigCode},
		"postinstall assertion":               {"--- FAIL: TestRetry (ETIMEDOUT expected)\nFAIL\n", "code", SigCode},
		"nothing recognised":                  {"something broke\n", "code", SigCode},
	} {
		d := ClassifyInstall(c.log)
		if d.Class != c.want || d.Signature != c.sig || (c.want == "infra") != d.Retryable {
			t.Errorf("%s: got %+v, want %s %s", name, d, c.want, c.sig)
		}
	}
}

const footer = "npm error A complete log of this run can be found in: /x/1.log\nnpm error line\nnpm error line\nnpm error line\n"
