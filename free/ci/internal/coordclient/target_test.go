package coordclient

// Purpose: tests of target resolution: the order flag > env > user config > live local serve > in-process,
// https only, the committed-config warning, the user config guards and the serve lock.
// Inputs:  temp directories, fake environments. Outputs: pass or fail.
// Constraints: no network; the lock is a real flock on a temp file.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
)

func env(kv ...string) func(string) string {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return func(k string) string { return m[k] }
}

func writeUserConfig(t *testing.T, home, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, "user.yaml"), []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(home, "user.yaml"), mode); err != nil {
		t.Fatal(err)
	}
}

func TestCoordinatorClientTargetOrder(t *testing.T) {
	home := t.TempDir()
	resolve := func(flagv string, e ...string) Target {
		t.Helper()
		tg, _, err := Resolve(Inputs{Flag: flagv, Getenv: env(e...), Home: home})
		if err != nil {
			t.Fatal(err)
		}
		return tg
	}
	if tg := resolve(""); tg.Kind != InProcess || tg.Active() {
		t.Fatalf("nothing configured must be in-process: %+v", tg)
	}
	release, err := AcquireServeLock(home)
	if err != nil {
		t.Fatal(err)
	}
	if tg := resolve(""); tg.Kind != LocalServe || tg.Socket != filepath.Join(home, "serve.sock") {
		t.Fatalf("a held serve lock must pick the local serve: %+v", tg)
	}
	writeUserConfig(t, home, "ci:\n  coordinator: https://cfg.example:3865\n  coordinator_ca: /x/root.pem\n", 0o600)
	if tg := resolve(""); tg.Kind != Remote || tg.URL != "https://cfg.example:3865" || tg.Creds.CA != "/x/root.pem" {
		t.Fatalf("the user config beats a local serve: %+v", tg)
	}
	if tg := resolve("", EnvCoordinator, "https://env.example"); tg.URL != "https://env.example" || tg.Creds.CA != "/x/root.pem" {
		t.Fatalf("the environment beats the user config: %+v", tg)
	}
	if tg := resolve("https://flag.example/", EnvCoordinator, "https://env.example"); tg.URL != "https://flag.example" {
		t.Fatalf("the flag beats the environment: %+v", tg)
	}
	release()
	_ = os.Remove(filepath.Join(home, "user.yaml"))
	if tg := resolve(""); tg.Kind != InProcess {
		t.Fatalf("a lock file nobody holds is stale, not a serve: %+v", tg)
	}
	if _, err = os.Stat(LockPath(home)); err != nil {
		t.Fatal("the stale lock file should still be there; resolving must not write or remove it")
	}
}

func TestCoordinatorClientServeLockIsExclusive(t *testing.T) {
	home := t.TempDir()
	release, err := AcquireServeLock(home)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err = AcquireServeLock(home); err == nil {
		t.Fatal("a second serve on the same home must be refused")
	}
	info, _ := os.Stat(LockPath(home))
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("lock mode %v", info.Mode().Perm())
	}
}

func TestCoordinatorClientAddressRules(t *testing.T) {
	for raw, reason := range map[string]string{
		"http://ci.example:3865":          clientapi.ReasonPlainHTTP,
		"ci.example:3865":                 clientapi.ReasonBadRequest,
		"https://user" + ":pw@ci.example": clientapi.ReasonBadRequest,
		"https://ci.example/path":         clientapi.ReasonBadRequest,
		"https://ci.example?x=1":          clientapi.ReasonBadRequest,
		"ftp://ci.example":                clientapi.ReasonPlainHTTP,
		"https://":                        clientapi.ReasonBadRequest,
	} {
		if _, err := ParseURL(raw); !reasonIs(err, reason) {
			t.Errorf("%q: %v, want %s", raw, err, reason)
		}
	}
	if u, err := ParseURL(" https://ci.example:3865/ "); err != nil || u != "https://ci.example:3865" {
		t.Fatalf("%q %v", u, err)
	}
}

func TestCoordinatorClientCommittedValueIsIgnoredWithOneWarning(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\nci:\n  coordinator: https://attacker.example\n  coordinator_token_file: /etc/passwd\n"
	if err := os.WriteFile(filepath.Join(repo, ".nself", "ci.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r := &Resolver{Warn: &buf, Inputs: func() Inputs { return Inputs{Getenv: env(), Home: home, RepoRoot: repo} }}
	for i := 0; i < 3; i++ { // several calls in one invocation print the warning once
		tg, err := r.Target()
		if err != nil || tg.Kind != InProcess {
			t.Fatalf("a committed ci.coordinator must not choose the coordinator: %+v %v", tg, err)
		}
	}
	if n := lineCount(buf.String()); n != 1 || !strings.Contains(buf.String(), "ignoring ci.coordinator") || strings.Contains(buf.String(), "attacker") {
		t.Fatalf("want exactly one warning that names the key and not the value, got %d: %q", n, buf.String())
	}
	if r.Active() {
		t.Fatal("an ignored value must leave the run in-process")
	}
}

func TestCoordinatorClientUserConfigGuards(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, "ci:\n  coordinator: https://x.example\n", 0o666)
	if _, _, err := Resolve(Inputs{Getenv: env(), Home: home}); !reasonIs(err, clientapi.ReasonCredentialFile) {
		t.Fatalf("a config anyone can write must be refused: %v", err)
	}
	writeUserConfig(t, home, "ci: [not, a, map", 0o600)
	if _, _, err := Resolve(Inputs{Getenv: env(), Home: home}); !reasonIs(err, clientapi.ReasonBadRequest) {
		t.Fatalf("invalid YAML: %v", err)
	}
	r := &Resolver{Warn: &bytes.Buffer{}, Inputs: func() Inputs { return Inputs{Getenv: env(), Home: home} }}
	if !r.Active() {
		t.Fatal("a broken configuration must be Active, so the work never silently runs on this machine")
	}
}
