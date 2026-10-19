package specs

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/detect"
)

func fact(kind, name, path string) detect.Fact {
	return detect.Fact{Kind: kind, Name: name, Path: path, Confidence: detect.Certain}
}

func ids(list []Spec) (out []string) {
	for _, s := range list {
		out = append(out, s.ID)
	}
	return out
}

func TestForSelectsSpecsFromDetectorFacts(t *testing.T) {
	cases := []struct {
		name      string
		facts     []detect.Fact
		container bool
		want      string
	}{
		{"go", []detect.Fact{fact("stack", "go", "go.mod")}, false, "go"},
		{"pnpm", []detect.Fact{fact("stack", "node", "package.json"), fact("package_manager", "pnpm", "package.json")}, false, "pnpm"},
		{"npm", []detect.Fact{fact("package_manager", "npm", "package.json")}, false, "npm"},
		{"cargo", []detect.Fact{fact("stack", "rust", "Cargo.toml")}, false, "cargo"},
		{"uv", []detect.Fact{fact("package_manager", "uv", "pyproject.toml")}, false, "uv"},
		{"pip", []detect.Fact{fact("stack", "python", "requirements.txt")}, false, "pip"},
		{"buildx only in a container", []detect.Fact{fact("container", "dockerfile", "Dockerfile")}, true, "buildx"},
		{"buildx not at process level", []detect.Fact{fact("container", "dockerfile", "Dockerfile")}, false, ""},
		{"unknown facts select nothing", []detect.Fact{{Kind: "stack", Name: "go", Path: "go.mod", Confidence: detect.Unknown}}, false, ""},
		{"yarn and bun have no spec", []detect.Fact{fact("package_manager", "yarn", "package.json"), fact("package_manager", "bun", "package.json")}, false, ""},
	}
	for _, c := range cases {
		if got := strings.Join(ids(For(c.facts, c.container)), ","); got != c.want {
			t.Errorf("%s: specs %q, want %q", c.name, got, c.want)
		}
	}
	if got := ids(For([]detect.Fact{fact("stack", "go", "go.mod"), fact("stack", "rust", "Cargo.toml"), fact("package_manager", "pnpm", "p.json")}, false)); strings.Join(got, ",") != "cargo,go,pnpm" {
		t.Fatalf("a polyglot repo selects its specs in id order, got %v", got)
	}
}

func TestNoSpecCachesABuildOutputDir(t *testing.T) {
	for _, id := range []string{"go", "pnpm", "npm", "cargo", "uv", "pip", "buildx"} {
		s, ok := Get(id)
		if !ok {
			t.Fatalf("spec %s is not registered", id)
		}
		for _, d := range s.Dirs {
			if strings.EqualFold(d.Name, "target") || strings.Contains(d.Name, "/") {
				t.Errorf("spec %s caches %q", id, d.Name)
			}
		}
	}
	cargo, _ := Get("cargo")
	if cargo.RootEnv != "CARGO_HOME" || len(cargo.Dirs) != 2 || cargo.Dirs[0].Name != "git" || cargo.Dirs[1].Name != "registry" {
		t.Fatalf("cargo caches the registry and git dirs only: %+v", cargo)
	}
}

func TestRegisterRejectsBadSpecs(t *testing.T) {
	ok := func([]detect.Fact) bool { return true }
	for name, s := range map[string]Spec{
		"empty id":      {Version: "1", Ecosystem: "x", Dirs: []Dir{{"a", ""}}, applies: ok},
		"uppercase id":  {ID: "Go", Version: "1", Ecosystem: "x", Dirs: []Dir{{"a", ""}}, applies: ok},
		"no version":    {ID: "z1", Ecosystem: "x", Dirs: []Dir{{"a", ""}}, applies: ok},
		"no dirs":       {ID: "z2", Version: "1", Ecosystem: "x", applies: ok},
		"target dir":    {ID: "z3", Version: "1", Ecosystem: "x", Dirs: []Dir{{"target", ""}}, applies: ok},
		"traversal dir": {ID: "z4", Version: "1", Ecosystem: "x", Dirs: []Dir{{"../x", ""}}, applies: ok},
		"repeated dir":  {ID: "z5", Version: "1", Ecosystem: "x", Dirs: []Dir{{"a", ""}, {"a", ""}}, applies: ok},
		"no rule":       {ID: "z6", Version: "1", Ecosystem: "x", Dirs: []Dir{{"a", ""}}},
		"duplicate id":  {ID: "go", Version: "1", Ecosystem: "x", Dirs: []Dir{{"a", ""}}, applies: ok},
	} {
		if err := Register(s); err == nil {
			t.Errorf("%s was registered", name)
		}
	}
}

func TestEnvDigestHashesTheAllowlistOnly(t *testing.T) {
	allow := []string{"GOFLAGS", "CARGO_*"}
	base := []string{"GOFLAGS=-mod=vendor", "CARGO_TERM_COLOR=never", "SECRET_TOKEN=tok-not-allowlisted", "PATH=/bin"}
	d := EnvDigest(allow, base)
	if len(d) != 64 || strings.Contains(d, "vendor") {
		t.Fatalf("digest %q", d)
	}
	same := []string{"PATH=/other", "CARGO_TERM_COLOR=never", "GOFLAGS=-mod=vendor", "SECRET_TOKEN=changed"}
	if EnvDigest(allow, same) != d {
		t.Fatal("order and non-allowlisted variables must not change the digest")
	}
	for name, env := range map[string][]string{"value": {"GOFLAGS=-mod=mod", "CARGO_TERM_COLOR=never"}, "prefix var": {"GOFLAGS=-mod=vendor", "CARGO_TERM_COLOR=always"},
		"extra prefix var": append(append([]string{}, base...), "CARGO_BUILD_JOBS=2"), "unset": {"CARGO_TERM_COLOR=never"}, "empty is not unset": {"GOFLAGS=", "CARGO_TERM_COLOR=never"}} {
		if EnvDigest(allow, env) == d {
			t.Errorf("%s did not change the digest", name)
		}
	}
	// length prefixes: ("a","bc") and ("ab","c") never collide
	if EnvDigest([]string{"A*"}, []string{"AB=c"}) == EnvDigest([]string{"A*"}, []string{"A=Bc"}) {
		t.Fatal("name/value boundary collision")
	}
}

func TestKeyMissesWhenAnyInputChanges(t *testing.T) {
	ns, _ := authz.CacheNamespace("/work/proj", "owner", authz.Protected, "", "")
	goSpec, _ := Get("go")
	base := Input{Spec: goSpec, Platform: "linux/amd64", Isolation: "process", Facts: goFacts, Environ: []string{"GOFLAGS=-mod=vendor"}, Probe: probeGo}
	key := func(mut func(*Input)) string {
		in := base
		mut(&in)
		m, err := Resolve(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		k, err := m.Key(ns)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	ref := key(func(*Input) {})
	if key(func(*Input) {}) != ref {
		t.Fatal("the key is not deterministic")
	}
	for name, mut := range map[string]func(*Input){
		"GOFLAGS": func(i *Input) { i.Environ = []string{"GOFLAGS=-race"} },
		"probed version": func(i *Input) {
			i.Probe = func(context.Context, []string) (string, error) { return "go version go1.28.0 linux/amd64", nil }
		},
		"isolation": func(i *Input) { i.Isolation, i.Image = "container", "sha256:"+strings.Repeat("b", 64) },
		"platform":  func(i *Input) { i.Platform = "linux/arm64" },
		"lockfile": func(i *Input) {
			i.Facts = []detect.Fact{goFacts[0], {Kind: "lockfile", Name: "go", Path: "go.sum", SHA256: strings.Repeat("c", 64), Confidence: detect.Certain}}
		},
		"narrowed paths": func(i *Input) { i.Scope = []string{"svc"} },
		"extra key":      func(i *Input) { i.ExtraKeys = []string{"v2"} },
	} {
		if key(mut) == ref {
			t.Errorf("changing %s did not change the key", name)
		}
	}
	imgA := key(func(i *Input) { i.Isolation, i.Image = "container", "sha256:"+strings.Repeat("b", 64) })
	if imgB := key(func(i *Input) { i.Isolation, i.Image = "container", "sha256:"+strings.Repeat("d", 64) }); imgA == imgB {
		t.Error("a new toolchain image digest must miss")
	}
	other, _ := authz.CacheNamespace("/work/other", "owner", authz.Protected, "", "")
	m, _ := Resolve(context.Background(), base)
	if k, _ := m.Key(other); k == ref {
		t.Error("another project's namespace must not share a key")
	}
}

func TestResolveFailsClosedWithoutToolchainIdentity(t *testing.T) {
	goSpec, _ := Get("go")
	failing := func(context.Context, []string) (string, error) { return "", errors.New("not found") }
	for name, in := range map[string]Input{
		"probe fails":             {Spec: goSpec, Isolation: "process", Probe: failing},
		"probe prints nothing":    {Spec: goSpec, Isolation: "process", Probe: func(context.Context, []string) (string, error) { return "\n", nil }},
		"no probe":                {Spec: goSpec, Isolation: "process"},
		"container without image": {Spec: goSpec, Isolation: "container", Probe: probeGo},
	} {
		if _, err := Resolve(context.Background(), in); !errors.Is(err, ErrUnavailable) {
			t.Errorf("%s: err %v", name, err)
		}
	}
}

func TestEnvironmentValuesNeverReachTheStore(t *testing.T) {
	e := newEnv(t)
	e.running("a1", ownerFacts)
	in := e.input("a1", ownerFacts)
	in.Environ = []string{"GOFLAGS=-tags=plantedvalue-8f3a2c91", "PIP_INDEX_URL=https://user:plantedpass-77aa@example.invalid/simple"}
	s := e.begin(in)
	putFile(t, s.items[0].Dirs["mod"]+"/m.txt", "x")
	s.Finalize(e.ctx, e.finalizing("a1"))
	if e.entryCount() != 1 {
		t.Fatal("no entry saved")
	}
	for _, f := range []string{"state.db", "state.db-wal"} {
		if b, err := readIfExists(e.home + "/" + f); err == nil && (strings.Contains(string(b), "plantedvalue") || strings.Contains(string(b), "plantedpass")) {
			t.Fatalf("%s holds an environment value", f)
		}
	}
}
