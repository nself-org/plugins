package deployguard

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReaderRefusesAnyCommandThatIsNotRead(t *testing.T) {
	for _, side := range []string{"write", "remote", "destructive", "", "READ"} {
		run := &FixtureRunner{HelpSide: side, HostsByRoot: map[string][]string{"/p": {FixtureHostV4}}}
		es, err := Reader{Run: run.Run}.Read(context.Background(), "/p")
		if ReasonOf(err) != ReasonSideEffect || es != nil {
			t.Errorf("side_effect %q: %v %v", side, es, err)
		}
		if len(run.Calls) != 1 || !strings.Contains(run.Calls[0], "help --json deploy targets") {
			t.Errorf("side_effect %q ran more than the help check: %v", side, run.Calls)
		}
	}
}

func TestReaderParsesTheContractAndRunsInTheRoot(t *testing.T) {
	run := &FixtureRunner{HelpSide: "read", HostsByRoot: map[string][]string{"/p": {FixtureHostV4, "ci@" + FixtureHostName + ":22"}}}
	es, err := Reader{Run: run.Run, Bin: "/opt/nself"}.Read(context.Background(), "/p")
	if err != nil {
		t.Fatal(err)
	}
	s := NewSet(es)
	if !s.MatchHost(FixtureHostV4).Matched || !s.MatchHost(FixtureHostName).Matched || len(s.View().Fingerprints) != 1 {
		t.Fatalf("entries %+v", es)
	}
	for _, c := range run.Calls {
		if !strings.HasPrefix(c, "/p|/opt/nself ") {
			t.Errorf("call %q", c)
		}
	}
}

func TestReaderToleratesNoiseAndRejectsBadDocuments(t *testing.T) {
	good := string(FixtureTargets(FixtureHostV4))
	help := string(FixtureHelp("read"))
	cases := map[string]struct {
		help, targets string
		hexit, texit  int
		want          string
	}{
		"stderr noise before json": {help, "warning: stale cache\n" + good, 0, 0, ""},
		"help exit":                {help, good, 2, 0, ReasonInventory},
		"targets exit":             {help, good, 0, 1, ReasonInventory},
		"no document":              {help, "plain text", 0, 0, ReasonInventory},
		"wrong command":            {help, strings.Replace(good, "deploy targets", "other", 1), 0, 0, ReasonInventory},
		"no targets key":           {help, `{"command":"deploy targets","data":{}}`, 0, 0, ReasonInventory},
		"bad row shape":            {help, `{"command":"deploy targets","data":{"targets":[{"host":5}]}}`, 0, 0, ReasonInventory},
		"help without entry":       {`{"data":{"commands":[]}}`, good, 0, 0, ReasonSideEffect},
		"help not json":            {"nope", good, 0, 0, ReasonSideEffect},
	}
	for name, c := range cases {
		n := 0
		run := func(_ context.Context, _ string, argv ...string) ([]byte, int, error) {
			n++
			if argv[1] == "help" {
				return []byte(c.help), c.hexit, nil
			}
			return []byte(c.targets), c.texit, nil
		}
		_, err := Reader{Run: run}.Read(context.Background(), "/p")
		if got := ReasonOf(err); got != c.want {
			t.Errorf("%s: reason %q (%v), want %q", name, got, err, c.want)
		}
	}
	if _, err := (Reader{}).Read(context.Background(), "/p"); ReasonOf(err) != ReasonInventory {
		t.Errorf("no runner: %v", err)
	}
	boom := func(context.Context, string, ...string) ([]byte, int, error) {
		return nil, -1, errors.New("exec failed")
	}
	if _, err := (Reader{Run: boom}).Read(context.Background(), "/p"); ReasonOf(err) != ReasonInventory || strings.Contains(err.Error(), "exec failed") {
		t.Errorf("runner error leaked or wrong reason: %v", err)
	}
}

func stageFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, ".nself"), 0o700))
	must(os.WriteFile(filepath.Join(root, ".nself", "control-plane.yaml"), []byte("version: 1\n"), 0o600))
	must(os.WriteFile(filepath.Join(root, ".nself", "other.yaml"), []byte("x"), 0o600))
	must(os.WriteFile(filepath.Join(root, ".env"), []byte("ENV=prod\n"), 0o600))
	must(os.WriteFile(filepath.Join(root, ".env.prod"), make([]byte, maxStaged), 0o600)) // exactly the cap: staged
	must(os.WriteFile(filepath.Join(root, ".environment"), []byte("no"), 0o600))
	must(os.WriteFile(filepath.Join(root, "main.go"), []byte("package x"), 0o600))
	return root
}

func TestStageCopiesOnlyTheInventoryFiles(t *testing.T) {
	root := stageFixture(t)
	shared := t.TempDir()
	if err := os.WriteFile(filepath.Join(shared, "env.staging"), []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(shared, "env.staging"), filepath.Join(root, ".env.staging")); err != nil { // resolved, as nself reads it
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(shared, "gone"), filepath.Join(root, ".env.dangling")); err != nil { // absent: skipped
		t.Fatal(err)
	}
	out, exit, err := FunnelRun(context.Background(), root, "ls", "-A", ".", ".nself")
	if err != nil || exit != 0 {
		t.Fatalf("run: %v exit %d out %q", err, exit, out)
	}
	got := strings.Fields(string(out))
	want := map[string]bool{".env": true, ".env.prod": true, ".env.staging": true, ".nself": true, "control-plane.yaml": true, ".:": true, ".nself:": true}
	for _, g := range got {
		if !want[g] {
			t.Errorf("staged %q (all: %q)", g, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("staged %q", got)
	}
	if out, exit, _ = FunnelRun(context.Background(), "", "echo", "hi"); exit != 0 || strings.TrimSpace(string(out)) != "hi" {
		t.Errorf("no-dir run: %q %d", out, exit)
	}
	if out, exit, err = FunnelRun(context.Background(), ""); err == nil || exit != -1 || out != nil {
		t.Errorf("empty argv: %v %d %q", err, exit, out)
	}
}

func TestStageFailsOnAFileItCannotStage(t *testing.T) {
	for name, mk := range map[string]func(root string) error{
		"over the cap": func(r string) error {
			return os.WriteFile(filepath.Join(r, ".env.big"), make([]byte, maxStaged+1), 0o600)
		},
		"a directory":   func(r string) error { return os.Mkdir(filepath.Join(r, ".env.d"), 0o700) },
		"link to a dir": func(r string) error { return os.Symlink(t.TempDir(), filepath.Join(r, ".env.link")) },
		"symlink loop":  func(r string) error { return os.Symlink(".env.loop", filepath.Join(r, ".env.loop")) },
	} {
		root := stageFixture(t)
		if err := mk(root); err != nil {
			t.Fatal(err)
		}
		if _, _, err := FunnelRun(context.Background(), root, "true"); err == nil {
			t.Errorf("%s: staged anyway", name)
		}
	}
}

func TestCopyOneRefusesToOverwrite(t *testing.T) {
	root := stageFixture(t)
	ws, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	if err = copyOne(root, ws, ".env"); err != nil {
		t.Fatal(err)
	}
	f, err := ws.Open(".env")
	if err != nil {
		t.Fatalf("nothing was staged: %v", err)
	}
	b, _ := io.ReadAll(f)
	_ = f.Close()
	if string(b) != "ENV=prod\n" {
		t.Fatalf("staged content %q", b)
	}
	if err = copyOne(root, ws, ".env"); err == nil {
		t.Fatal("a second copy overwrote the staged file")
	}
	if err = copyOne(root, ws, ".nself/control-plane.yaml"); err != nil {
		t.Fatalf("nested copy: %v", err)
	}
	if err = copyOne(root, ws, ".env.absent"); err != nil {
		t.Fatalf("absent file is skipped: %v", err)
	}
}

func TestNeverListEntryKinds(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	text := "ci:\n  never_hosts:\n    - 203.0.113.0/24\n    - 2001:db8::/32\n    - " + FixtureFingerprint + "\n    - machine-id:" + FixtureMachineID + "\n    - Prod.Example.Test.\n    - ''\n"
	if err := os.WriteFile(cfg, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	es, err := LoadNeverList(cfg)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, e := range es {
		kinds[e.Kind]++
		if e.Source != SourceNeverList {
			t.Errorf("source %q", e.Source)
		}
	}
	if kinds[KindPrefix] != 2 || kinds[KindFingerprint] != 1 || kinds[KindMachineID] != 1 || kinds[KindHost] != 1 || len(es) != 5 {
		t.Fatalf("kinds %v", kinds)
	}
	if none, err := LoadNeverList(filepath.Join(t.TempDir(), "absent.yaml")); err != nil || none != nil {
		t.Fatalf("absent file: %v %v", none, err)
	}
	if err = os.WriteFile(cfg, make([]byte, 1<<20+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadNeverList(cfg); ReasonOf(err) != ReasonInventory {
		t.Fatalf("huge file: %v", err)
	}
	if p := ConfigPath(""); !strings.HasSuffix(p, "config.yaml") {
		t.Fatalf("default path %q", p)
	}
	t.Setenv("NSELF_CI_HOME", "/h")
	if ConfigPath("") != "/h/config.yaml" {
		t.Fatal("NSELF_CI_HOME ignored")
	}
}
