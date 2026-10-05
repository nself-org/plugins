package links

import (
	"os"
	"path/filepath"
	"testing"
)

func home(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	return h
}

func TestSaveLoadPerms(t *testing.T) {
	h := home(t)
	in := map[string]string{"b": "/two", "a": "/one"}
	if err := Save(in); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(h, ".nself", "plugin-links.json"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("stat %v mode %v", err, st)
	}
	data, _ := os.ReadFile(Path())
	if string(data) != "{\n  \"a\": \"/one\",\n  \"b\": \"/two\"\n}" {
		t.Fatalf("unexpected json %q", data)
	}
	out, err := Load()
	if err != nil || out["a"] != "/one" || out["b"] != "/two" {
		t.Fatalf("load %v %v", out, err)
	}
}

func TestLoadMissingAndBad(t *testing.T) {
	h := home(t)
	if m, err := Load(); err != nil || len(m) != 0 {
		t.Fatalf("missing: %v %v", m, err)
	}
	_ = os.MkdirAll(filepath.Join(h, ".nself"), 0o750)
	_ = os.WriteFile(Path(), []byte("{nope"), 0o600)
	if _, err := Load(); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestResolveName(t *testing.T) {
	d := t.TempDir()
	if n, _ := ResolveName(d); n != filepath.Base(d) {
		t.Fatalf("basename fallback: %q", n)
	}
	_ = os.WriteFile(filepath.Join(d, "plugin.yaml"), []byte("version: 1\nname: 'quoted'\n"), 0o600)
	if n, _ := ResolveName(d); n != "quoted" {
		t.Fatalf("yaml name: %q", n)
	}
}

func TestValidatePath(t *testing.T) {
	d := t.TempDir()
	if _, err := ValidatePath(filepath.Join(d, "none")); err == nil {
		t.Fatal("missing path accepted")
	}
	f := filepath.Join(d, "f")
	_ = os.WriteFile(f, nil, 0o600)
	if _, err := ValidatePath(f); err == nil {
		t.Fatal("file accepted")
	}
	if _, err := ValidatePath(d); err == nil {
		t.Fatal("dir without plugin.yaml accepted")
	}
	_ = os.WriteFile(filepath.Join(d, "plugin.yaml"), nil, 0o600)
	if got, err := ValidatePath(d); err != nil || got != d {
		t.Fatalf("valid dir: %q %v", got, err)
	}
}

func TestResolvePluginPathOrder(t *testing.T) {
	h := home(t)
	cwd := t.TempDir()
	t.Chdir(cwd)
	if _, err := ResolvePluginPath("x"); err == nil {
		t.Fatal("expected not found")
	}
	linked := filepath.Join(h, "linked")
	_ = os.MkdirAll(linked, 0o750)
	_ = Save(map[string]string{"x": linked})
	if got, _ := ResolvePluginPath("x"); got != linked {
		t.Fatalf("registry: %q", got)
	}
	_ = os.WriteFile(filepath.Join(cwd, "plugin.yaml"), nil, 0o600)
	if got, _ := ResolvePluginPath("x"); got != mustGetwd(t) {
		t.Fatalf("cwd plugin.yaml: %q", got)
	}
	_ = os.MkdirAll(filepath.Join(cwd, "x"), 0o750)
	if got, _ := ResolvePluginPath("x"); got != filepath.Join(mustGetwd(t), "x") {
		t.Fatalf("cwd/<name>: %q", got)
	}
}

func TestLinkAutoRegisters(t *testing.T) {
	home(t)
	d := t.TempDir()
	_ = os.WriteFile(filepath.Join(d, "plugin.yaml"), []byte("name: auto\n"), 0o600)
	if err := Link(d); err != nil {
		t.Fatal(err)
	}
	m, _ := Load()
	if m["auto"] != d {
		t.Fatalf("links %v", m)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}
