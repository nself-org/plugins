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
		{"go module proxy timeout", Input{ExecClass: "code", ExitCode: 1, Tail: "go: github.com/x/y@v1.0.0: Get \"https://proxy.golang.org/github.com/x/y/@v/v1.0.0.mod\": dial tcp 142.250.1.1:443: i/o timeout\n"}, "infra", SigNetwork},
		{"git cannot reach github", Input{ExecClass: "code", ExitCode: 128, Tail: "fatal: unable to access 'https://github.com/org/repo.git/': Connection timed out after 300001 milliseconds\n"}, "infra", SigNetwork},
		{"tls handshake to the docker registry", Input{ExecClass: "code", ExitCode: 1, Tail: "Get \"https://registry-1.docker.io/v2/\": net/http: TLS handshake timeout\n"}, "infra", SigNetwork},
		{"executor error carries it", Input{ExecClass: "infra", ExitCode: -1, Err: "Get \"https://proxy.golang.org/x\": read tcp 10.0.0.2:5->10.0.0.9:443: i/o timeout"}, "infra", SigNetwork},
		{"a test that curls its own server is code", Input{ExecClass: "code", ExitCode: 56, Tail: "curl: (56) Recv failure: Connection reset by peer\n"}, "code", SigCode},
		{"a reset with no provider is code", Input{ExecClass: "code", ExitCode: 1, Tail: "read tcp 10.0.0.2:5->10.0.0.9:443: connection reset by peer\n"}, "code", SigCode},
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
	npmNet := func(code, syscall, reason string) string {
		return "npm error code " + code + "\nnpm error syscall " + syscall + "\nnpm error errno " + code + "\nnpm error network request to https://registry.npmjs.org/left-pad failed, reason: " + reason + "\nnpm error network This is a problem related to network connectivity.\nnpm error network In most cases you are behind a proxy or have bad network settings.\n" + footer
	}
	for name, c := range map[string]struct {
		log  string
		want model.FailureClass
		sig  string
	}{
		// infrastructure: the manager's own structured error names the cause
		"npm enotfound":                        {npmNet("ENOTFOUND", "getaddrinfo", "getaddrinfo ENOTFOUND registry.npmjs.org"), "infra", SigDNS},
		"npm eai_again":                        {npmNet("EAI_AGAIN", "getaddrinfo", "getaddrinfo EAI_AGAIN registry.npmjs.org"), "infra", SigDNS},
		"npm etimedout":                        {npmNet("ETIMEDOUT", "connect", "connect ETIMEDOUT 104.16.0.35:443"), "infra", SigNetwork},
		"npm econnreset":                       {npmNet("ECONNRESET", "read", "read ECONNRESET"), "infra", SigNetwork},
		"npm econnrefused":                     {npmNet("ECONNREFUSED", "connect", "connect ECONNREFUSED 104.16.0.35:443"), "infra", SigNetwork},
		"npm enospc":                           {"npm error code ENOSPC\nnpm error syscall write\nnpm error errno -28\nnpm error nospc ENOSPC: no space left on device, write\n" + footer, "infra", SigDisk},
		"pnpm fetch fail":                      {" ERR_PNPM_META_FETCH_FAIL  GET https://registry.npmjs.org/left-pad: Client network socket disconnected before secure TLS connection was established\n", "infra", SigNetwork},
		"pnpm 503":                             {" ERR_PNPM_FETCH_503  GET https://registry.npmjs.org/left-pad/-/left-pad-1.0.0.tgz: Service Unavailable - 503\n", "infra", SigNetwork},
		"yarn classic enotfound":               {"info There appears to be trouble with your network connection. Retrying...\nerror An unexpected error occurred: \"https://registry.yarnpkg.com/left-pad: getaddrinfo ENOTFOUND registry.yarnpkg.com\".\n", "infra", SigDNS},
		"yarn classic hang up":                 {"error An unexpected error occurred: \"https://registry.yarnpkg.com/left-pad: socket hang up\".\n", "infra", SigNetwork},
		"yarn berry reset":                     {"➤ YN0001: │ Error: connect ECONNRESET 104.16.0.35:443\n", "infra", SigNetwork},
		"bun refused":                          {"error: ConnectionRefused downloading package manifest left-pad\n", "infra", SigNetwork},
		"retry warnings dropped":               {"npm warn ECONNRESET retrying\nnpm warn ECONNRESET retrying\nnpm error code ENOSPC\n" + footer, "infra", SigDisk},
		"fetched fine, then disk":              {"npm http fetch GET 200 https://registry.npmjs.org/etimedout 15ms\nnpm error code ENOSPC\n", "infra", SigDisk},
		"enoent path named enospc":             {"npm error code ENOENT\nnpm error syscall open\nnpm error path /repo/enospc/package.json\nnpm error errno -2\nnpm error enoent ENOENT: no such file or directory, open '/repo/enospc/package.json'\n", "code", SigCode},
		"local registry url":                   {"npm error code ECONNRESET\nnpm error network request to http://localhost:4873/left-pad failed, reason: socket hang up\n", "code", SigCode},
		"local registry ipv6":                  {"npm error code ECONNRESET\nnpm error network request to http://[::1]:4873/left-pad failed, reason: socket hang up\n", "code", SigCode},
		"yarn error for a package called warn": {"error An unexpected error occurred: \"https://registry.yarnpkg.com/warn: socket hang up\".\n", "infra", SigNetwork},
		"shell kill":                           {"sh: line 1:  4242 Killed                  npm ci\n", "infra", SigOOM},
		// code: everything else
		"nothing recognised":                        {"something broke\n", "code", SigCode},
		"sync (npm)":                                {"npm error `npm ci` can only install packages when your package.json and package-lock.json are in sync.\nnpm error Missing: x@1.0.0 from lock file\n", "code", SigCode},
		"sync after a transient":                    {"npm warn ECONNRESET retrying\nERR_PNPM_OUTDATED_LOCKFILE  not up to date\n", "code", SigCode},
		"sync beyond 60 lines":                      {"ERR_PNPM_OUTDATED_LOCKFILE  not up to date\n" + strings.Repeat("detail line\n", 70) + "npm error code ECONNRESET\n", "code", SigCode},
		"yarn berry sync":                           {"YN0028: The lockfile would have been modified by this install\n", "code", SigCode},
		"yarn classic sync":                         {"error Your lockfile needs to be updated, but yarn was run with `--frozen-lockfile`.\n", "code", SigCode},
		"bun sync":                                  {"error: lockfile had changes, but lockfile is frozen\n", "code", SigCode},
		"pnpm 404":                                  {" ERR_PNPM_FETCH_404  GET https://registry.npmjs.org/nope: Not Found - 404\n", "code", SigCode},
		"npm 404 after a reset":                     {"npm error code ECONNRESET\nnpm error code E404\nnpm error 404 Not Found - GET https://registry.npmjs.org/enospc\n", "code", SigCode},
		"403 for a package etimedout":               {"npm error code E403\nnpm error 403 403 Forbidden - GET https://registry.npmjs.org/etimedout\n", "code", SigCode},
		"yarn engine message":                       {"error enotfound@1.0.0: The engine \"node\" is incompatible with this module. Expected version \">=99\". Got \"22.1.0\"\nerror Found incompatible module.\ninfo Visit https://yarnpkg.com/en/docs/cli/install for documentation about this command.\n", "code", SigCode},
		"postinstall assertion":                     {"--- FAIL: TestRetry (ETIMEDOUT expected)\nFAIL\n", "code", SigCode},
		"node assertion":                            {"AssertionError [ERR_ASSERTION]: expected ECONNREFUSED to be handled\nnpm error code ECONNREFUSED\n", "code", SigCode},
		"postinstall db":                            {"> app@1.0.0 postinstall\n> node scripts/migrate.js\nError: connect ECONNREFUSED 127.0.0.1:5432\nnpm error command failed\n", "code", SigCode},
		"local refused, no script":                  {"npm error code ECONNREFUSED\nnpm error connect ECONNREFUSED 127.0.0.1:4873\nnpm error network This is a problem related to network connectivity.\n", "code", SigCode},
		"warned transient then a build error":       {"npm warn ECONNRESET retrying\n> app@1.0.0 postinstall\nsrc/a.ts(1,1): error TS2322: bad\nnpm error command failed\n", "code", SigCode},
		"yarn classic lifecycle":                    {"error /app/node_modules/x: Command failed.\nExit code: 1\nCommand: node install.js\nOutput:\ngetaddrinfo ENOTFOUND project-db.internal\n", "code", SigCode},
		"pnpm retry then gyp":                       {"WARN  GET https://r/x error (ETIMEDOUT). Will retry in 10 seconds. 2 retries left.\ngyp ERR! build error\n ELIFECYCLE  Command failed with exit code 1.\n", "code", SigCode},
		"a package called localhost, then disk":     {"npm http fetch GET 200 https://registry.npmjs.org/localhost 9ms\nnpm error code ENOSPC\n", "infra", SigDisk},
		"a package called unauthorized, then reset": {"npm http fetch GET 200 https://registry.npmjs.org/unauthorized 9ms\nnpm error code ECONNRESET\nnpm error syscall read\n", "infra", SigNetwork},
	} {
		d := ClassifyInstall(c.log)
		if d.Class != c.want || d.Signature != c.sig || (c.want == "infra") != d.Retryable {
			t.Errorf("%s: got %+v, want %s %s", name, d, c.want, c.sig)
		}
	}
}

const footer = "npm error A complete log of this run can be found in: /x/1.log\n"
