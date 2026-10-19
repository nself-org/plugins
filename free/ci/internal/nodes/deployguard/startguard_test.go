package deployguard

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

type fsMap map[string]string

func (m fsMap) read(p string) ([]byte, error) {
	if v, ok := m[p]; ok {
		return []byte(v), nil
	}
	return nil, os.ErrNotExist
}

func localOn(host string, addr string, files fsMap, docker RunFunc) Local {
	return Local{
		Hostname: func() (string, error) { return host, nil },
		Addrs: func() ([]netip.Addr, error) {
			if addr == "" {
				return nil, nil
			}
			return []netip.Addr{netip.MustParseAddr(addr)}, nil
		},
		ReadFile: files.read, Glob: func(string) ([]string, error) { return []string{"/etc/ssh/ssh_host_ed25519_key.pub"}, nil },
		Run: docker, MachineIDs: []string{"/etc/machine-id"}, HostKeys: "/etc/ssh/ssh_host_*_key.pub",
	}
}

func dockerOut(out string, exit int) RunFunc {
	return func(_ context.Context, _ string, argv ...string) ([]byte, int, error) {
		if len(argv) < 2 || argv[0] != "docker" || argv[1] != "ps" {
			return nil, -1, errors.New("unexpected command")
		}
		return []byte(out), exit, nil
	}
}

const githubKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl github"
const githubFP = "SHA256:+DiY3wvvV6TuJJhbpZisF/zLDA0zPMSvHdkr4UvCOqU"

func TestPublicKeyFingerprintKnownVector(t *testing.T) {
	if got := PublicKeyFingerprint(githubKey); got != githubFP {
		t.Fatalf("%s", got)
	}
	for _, bad := range []string{"", "ssh-ed25519", "ssh-ed25519 !!!notbase64", "ssh-ed25519 "} {
		if PublicKeyFingerprint(bad) != "" {
			t.Errorf("%q", bad)
		}
	}
}

func TestStartGuardStopsOnEveryLocalIdentity(t *testing.T) {
	files := fsMap{"/etc/ssh/ssh_host_ed25519_key.pub": githubKey + "\n", "/etc/machine-id": FixtureMachineID + "\n"}
	cases := map[string]struct {
		set  []Entry
		host string
		addr string
	}{
		"hostname":    {[]Entry{{KindHost, "BOX.example.test.", SourceInventory}}, "box.example.test", ""},
		"address":     {[]Entry{{KindHost, FixtureHostV4, SourceInventory}}, "other", FixtureHostV4},
		"mapped addr": {[]Entry{{KindHost, FixtureHostV4, SourceInventory}}, "other", "::ffff:" + FixtureHostV4},
		"prefix":      {[]Entry{{KindPrefix, "203.0.113.0/24", SourceNeverList}}, "other", "203.0.113.200"},
		"host key":    {[]Entry{{KindFingerprint, githubFP, SourceInventory}}, "other", ""},
		"machine id":  {[]Entry{{KindMachineID, FixtureMachineID, SourceNeverList}}, "other", ""},
	}
	for name, c := range cases {
		l := localOn(c.host, c.addr, files, nil)
		_, err := l.Check(context.Background(), NewSet(c.set), "nself-ci-agent serve")
		var e *Error
		if !errors.As(err, &e) || e.Code != "E663" || e.Reason != ReasonMatched || !strings.Contains(e.Error(), "nself-ci-agent serve") {
			t.Errorf("%s: %v", name, err)
		}
	}
	l := localOn("calm.example.test", "192.0.2.4", fsMap{"/etc/machine-id": "ffffffffffffffffffffffffffffffff"}, nil)
	if w, err := l.Check(context.Background(), fixtureSet(), "serve"); err != nil || len(w) != 0 {
		t.Fatalf("clean host: %v %v", w, err)
	}
}

func pathWith(t *testing.T, docker bool) {
	t.Helper()
	dir := t.TempDir()
	if docker {
		if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "notexec"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestOnPath(t *testing.T) {
	pathWith(t, true)
	if !onPath("docker") || onPath("notexec") || onPath("absent") {
		t.Fatal("onPath")
	}
	pathWith(t, false)
	if onPath("docker") {
		t.Fatal("docker found in an empty PATH")
	}
}

func TestStartGuardProdMarker(t *testing.T) {
	dir := "/srv/stack"
	prod := fsMap{dir + "/.env": "FOO=1\nENV=production # live\n"}
	boom := func(context.Context, string, ...string) ([]byte, int, error) { return nil, -1, errors.New("timeout") }
	cases := map[string]struct {
		files  fsMap
		docker RunFunc
		path   bool
		reason string
		warn   bool
	}{
		"prod .env":          {prod, dockerOut(dir+"\n", 0), true, ReasonMarker, false},
		"prod .env.prod":     {fsMap{dir + "/.env.prod": "export ENV='prod'"}, dockerOut("\n"+dir+"\n"+dir+"\n", 0), true, ReasonMarker, false},
		"dev env":            {fsMap{dir + "/.env": "ENV=dev\n"}, dockerOut(dir, 0), true, "", false},
		"no env file":        {fsMap{}, dockerOut(dir, 0), true, "", false},
		"relative dir":       {prod, dockerOut("stack", 0), true, "", false},
		"no compose label":   {prod, dockerOut("\n\n", 0), true, "", false},
		"docker not on PATH": {prod, dockerOut(dir, 0), false, "", false},
		"docker unreadable":  {prod, dockerOut("permission denied", 1), true, "", true},
		"docker run failed":  {prod, boom, true, "", true},
		"no runner":          {prod, nil, true, "", false},
	}
	for name, c := range cases {
		pathWith(t, c.path)
		l := localOn("h", "", c.files, c.docker)
		w, err := l.Check(context.Background(), Set{}, "ci serve")
		if got := ReasonOf(err); got != c.reason {
			t.Errorf("%s: reason %q (%v)", name, got, err)
		}
		if (len(w) > 0) != c.warn {
			t.Errorf("%s: warnings %v", name, w)
		}
	}
}

func TestEnvSaysProd(t *testing.T) {
	for in, want := range map[string]bool{
		"ENV=prod": true, "ENV=Production": true, "export ENV=\"prod\"": true, " ENV = prod ": true, "ENV=prod # c": true,
		"ENV=staging": false, "NODE_ENV=prod": false, "ENV=": false, "# ENV=prod": false, "ENVIRONMENT=prod": false, "": false,
	} {
		if got := envSaysProd(in); got != want {
			t.Errorf("%q = %v", in, got)
		}
	}
}

func TestStartGuardBuildsItsSetFromStateDBAndNeverList(t *testing.T) {
	home := t.TempDir()
	ctx := context.Background()
	files := fsMap{}
	calm := localOn("calm.example.test", FixtureHostV4, files, nil)
	// No state.db and no config: nothing excluded, and no state.db is created.
	if w, err := StartGuard(ctx, home, calm, "serve"); err != nil || len(w) != 0 {
		t.Fatalf("empty home: %v %v", w, err)
	}
	if _, err := os.Stat(filepath.Join(home, "state.db")); !os.IsNotExist(err) {
		t.Fatal("the guard created a state.db")
	}
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.ExclusionReplace(ctx, "/p", []store.ExclusionEntry{{Root: "/p", Kind: "host", Value: FixtureHostV4, Source: "inventory"}}); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	_, err = StartGuard(ctx, home, calm, "join")
	if ReasonOf(err) != ReasonMatched {
		t.Fatalf("cached inventory ignored: %v", err)
	}
	other := localOn("calm.example.test", "198.51.100.1", files, nil)
	if err = os.WriteFile(filepath.Join(home, "config.yaml"), []byte("ci:\n  never_hosts: [calm.example.test]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = StartGuard(ctx, home, other, "pull"); ReasonOf(err) != ReasonMatched {
		t.Fatalf("never-list ignored: %v", err)
	}
	// A config that exists but cannot be parsed refuses the start: the guard never runs on less than the
	// operator wrote down.
	if err = os.WriteFile(filepath.Join(home, "config.yaml"), []byte("ci: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if w, err := StartGuard(ctx, home, other, "pull"); ReasonOf(err) != ReasonNoIdentity || w != nil || !strings.Contains(err.Error(), "never-list") {
		t.Fatalf("broken config: %v %v", w, err)
	}
	if err = os.Remove(filepath.Join(home, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	// So does a state.db that exists but is not a database.
	if err = os.WriteFile(filepath.Join(home, "state.db"), []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = StartGuard(ctx, home, other, "pull"); ReasonOf(err) != ReasonNoIdentity || !strings.Contains(err.Error(), "state database") {
		t.Fatalf("broken state: %v", err)
	}
}

func TestNewLocalReadsTheRealHost(t *testing.T) {
	l := NewLocal(nil)
	f, _ := l.Facts()
	if len(f.Names) != 1 || f.Names[0] == "" {
		t.Fatalf("hostname missing: %+v", f)
	}
	if _, err := readSmall(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("absent file read")
	}
	big := filepath.Join(t.TempDir(), "big")
	if err := os.WriteFile(big, make([]byte, 1<<20+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSmall(big); err == nil {
		t.Fatal("huge file read")
	}
}
