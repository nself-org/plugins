package internal

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func tailLines(r io.ReadSeeker, n int) ([]string, error) {
	// Read all content and return the last n lines. Log files for a
	// self-hosted runner are small enough that a full read is fine.
	scanner := bufio.NewScanner(r)
	var all []string
	for scanner.Scan() {
		all = append(all, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	if len(all) <= n {
		return all, nil
	}
	return all[len(all)-n:], nil
}

// --------------------------------------------------------------------------
// Path helpers
// --------------------------------------------------------------------------

// runnerDir returns the directory where the runner binary and config live.
func runnerDir() string {
	if d := os.Getenv("GITHUB_RUNNER_DIR"); d != "" {
		return d
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "/root"
	}
	return filepath.Join(home, defaultRunnerDir)
}

// logFilePath returns the path to the runner log file.
func logFilePath() string {
	if p := os.Getenv("GITHUB_RUNNER_LOG_PATH"); p != "" {
		return p
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "/root"
	}
	return filepath.Join(home, ".nself", "logs", "plugins", "github-runner", "runner.log")
}

// --------------------------------------------------------------------------
// Subprocess environment
// --------------------------------------------------------------------------

// runnerEnvAllowExact is the set of base environment variables the runner
// process legitimately needs. Everything else is dropped.
var runnerEnvAllowExact = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true,
	"SHELL": true, "TERM": true, "TZ": true, "TMPDIR": true,
	"LANG": true, "PWD": true,
}

// runnerEnvAllowPrefixes are name prefixes that are safe to forward: locale
// vars and the runner's own GitHub/RUNNER configuration.
var runnerEnvAllowPrefixes = []string{
	"LC_",
	"GITHUB_RUNNER_",
	"RUNNER_",
}

// runnerEnvDenySubstrings are substrings that mark a variable as secret-bearing.
// Any variable whose (upper-cased) name contains one of these is dropped even if
// it would otherwise match an allow rule — a hard deny that wins over allows.
var runnerEnvDenySubstrings = []string{
	"HASURA", "DATABASE", "NSELF_LICENSE", "STRIPE",
	"SECRET", "TOKEN", "KEY", "PASSWORD", "PASSWD",
	"CREDENTIAL", "PRIVATE", "PAT",
}

// runnerEnv builds the minimal, secret-free environment passed to the GitHub
// runner subprocess. The Actions runner executes untrusted workflow code, so it
// must NOT inherit the plugin's full os.Environ() — that would expose
// HASURA_ADMIN_SECRET, DATABASE_URL, NSELF_LICENSE_PRIV_HEX, Stripe keys, etc.
// Rules: a hard deny-list (secret-bearing substrings) wins over an allow-list of
// exact names and safe prefixes.
//
// Inputs:    process os.Environ() (read internally).
// Outputs:   filtered []string of "KEY=VALUE" entries.
// Constraints: deny always beats allow; unknown vars are dropped by default.
func runnerEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		name := kv[:eq]
		upper := strings.ToUpper(name)

		denied := false
		for _, d := range runnerEnvDenySubstrings {
			if strings.Contains(upper, d) {
				denied = true
				break
			}
		}
		if denied {
			continue
		}

		if runnerEnvAllowExact[name] {
			out = append(out, kv)
			continue
		}
		for _, p := range runnerEnvAllowPrefixes {
			if strings.HasPrefix(name, p) {
				out = append(out, kv)
				break
			}
		}
	}
	return out
}
