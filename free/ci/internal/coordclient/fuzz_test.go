package coordclient

// Purpose: fuzz targets for the client's untrusted input: the coordinator address, the user config file
// and the committed repository config (P7-NODE-19, CRITICAL tier).
// Inputs:  random bytes. Outputs: pass or fail; seeds run in every `go test`.
// Constraints: no panic; an accepted address is always https with a host and no user info, query or path;
//          a committed config can never produce a target.

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzParseURL(f *testing.F) {
	for _, s := range []string{"https://ci.example:3865", "http://ci.example", "https://u" + ":p@h", "https://h/p", "https://h?q", "ftp://h", "", " https://h ", "https://[::1]:3865", "https://h\x00", "//h"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		got, err := ParseURL(raw)
		if err != nil {
			return
		}
		u, perr := url.Parse(got)
		if perr != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
			t.Fatalf("%q was accepted as %q", raw, got)
		}
	})
}

func FuzzUserConfigAndCommittedConfig(f *testing.F) {
	for _, s := range []string{"ci:\n  coordinator: https://x\n", "ci: [", "", "ci:\n  coordinator: 1\n", "\x00\x01", "ci:\n  coordinator_ca: " + strings.Repeat("a", 4000)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, doc string) {
		home, repo := t.TempDir(), t.TempDir()
		_ = os.WriteFile(filepath.Join(home, "user.yaml"), []byte(doc), 0o600)
		_ = os.MkdirAll(filepath.Join(repo, ".nself"), 0o755)
		_ = os.WriteFile(filepath.Join(repo, ".nself", "ci.yaml"), []byte(doc), 0o644)
		// the committed file alone must never yield a target
		tg, _, err := Resolve(Inputs{Getenv: env(), Home: t.TempDir(), RepoRoot: repo})
		if err != nil || tg.Kind != InProcess {
			t.Fatalf("a committed config produced %+v (%v) from %q", tg, err, doc)
		}
		_, _, _ = Resolve(Inputs{Getenv: env(), Home: home}) // the user file may be refused, never panic
	})
}
