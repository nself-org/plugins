package affected

// Regression tests for the review rounds of P7-SCHED-13 (each case is a
// concrete input that once selected fewer jobs than a correct run needs).

import (
	"io/fs"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

func buildFS(t *testing.T, m fstest.MapFS) *Graph {
	t.Helper()
	g, err := Build(m)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func selectFor(t *testing.T, m fstest.MapFS, jobs []plan.JobRef, changed ...string) (model.Selection, []string) {
	t.Helper()
	g := buildFS(t, m)
	return Selector(plan.SelectionInput{Changed: changed, BaseKnown: true, Jobs: jobs, Facts: g.Facts()})
}

func TestModuleResolutionIsPerImportingModule(t *testing.T) {
	m := fstest.MapFS{
		"app/go.mod":                 mf("module example.com/app\n\nrequire example.com/lib v0.0.0\n\nreplace example.com/lib => \"../lib\"\n"),
		"app/main.go":                mf("package main\nimport _ \"example.com/lib/sub\"\nimport _ \"example.com/vendored/x\"\n"),
		"lib/go.mod":                 mf("module example.com/lib\n"),
		"lib/sub/x.go":               mf("package sub\n"),
		"shadow/go.mod":              mf("module example.com/lib/sub\n"),
		"shadow/s.go":                mf("package shadow\n"),
		"other/go.mod":               mf("module example.com/other\n\nreplace example.com/vendored => ./third_party/v\n"),
		"other/o.go":                 mf("package other\nimport _ \"example.com/vendored/x\"\n"),
		"other/third_party/v/x/x.go": mf("package x\n"),
		"unrelated/go.mod":           mf("module example.com/unrelated\n"),
		"unrelated/u.go":             mf("package u\nimport _ \"example.com/lib/sub\"\n"), // not replaced here: a released version
	}
	g := buildFS(t, m)
	want := []string{"app->lib/sub", "other->other/third_party/v/x"}
	if got := edgesOf(g); !reflect.DeepEqual(got, want) {
		t.Fatalf("edges = %v, want %v (reasons %v)", got, want, g.reasons)
	}
	if len(g.reasons) != 0 {
		t.Fatalf("uncertain: %v", g.reasons)
	}
}

func TestGoWorkUseMakesSiblingsVisible(t *testing.T) {
	m := fstest.MapFS{
		"go.work":  mf("go 1.22\n\nuse (\n\t./a\n\t./b\n)\n"),
		"a/go.mod": mf("module example.com/a\n"),
		"a/a.go":   mf("package a\n"),
		"b/go.mod": mf("module example.com/b\n"),
		"b/b.go":   mf("package b\nimport _ \"example.com/a\"\n"),
		"c/go.mod": mf("module example.com/c\n"),
		"c/c.go":   mf("package c\nimport _ \"example.com/a\"\n"), // c is not in the workspace
	}
	if got, want := edgesOf(buildFS(t, m)), []string{"b->a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("edges = %v, want %v", got, want)
	}
}

func TestSelectsEveryCoveringWorkdir(t *testing.T) {
	m := fstest.MapFS{"svc/go.mod": mf("module s\n"), "svc/a/a.go": mf("package a\n"), "svc/api/migrations/1.sql": mf("x")}
	jobs := []plan.JobRef{{ID: "svc", Workdir: "svc"}, {ID: "api", Workdir: "svc/api"}}
	sel, got := selectFor(t, m, jobs, "svc/api/migrations/1.sql")
	if sel.Mode != "affected" || !reflect.DeepEqual(got, []string{"api", "svc"}) {
		t.Fatalf("got %v %v", sel, got)
	}
}

func TestRootFilesAreUnmapped(t *testing.T) {
	m := fstest.MapFS{
		"package.json":        mf(`{"name":"root","packageManager":"pnpm@9"}`),
		"pnpm-workspace.yaml": mf("packages: ['web/*']\n"),
		"web/a/package.json":  mf(`{"name":"a"}`),
		"tsconfig.base.json":  mf("{}"),
		"turbo.json":          mf("{}"),
		"main.go":             mf("package main\n"),
		"assets/x.json":       mf("{}"),
		".golangci.yml":       mf("x: 1\n"),
	}
	jobs := []plan.JobRef{{ID: "lint", Workdir: "."}, {ID: "a.test", Workdir: "web/a"}}
	for _, p := range []string{"tsconfig.base.json", "turbo.json", ".golangci.yml", "assets/x.json"} {
		if sel, got := selectFor(t, m, jobs, p); sel.Mode != "full" || len(got) != 2 {
			t.Errorf("%s: %v %v, want full", p, sel, got)
		}
	}
	if sel, got := selectFor(t, m, jobs, "main.go"); sel.Mode != "affected" || !reflect.DeepEqual(got, []string{"lint"}) {
		t.Errorf("main.go: %v %v, want only the root job", sel, got)
	}
}

func TestRootPackageEmbedSeedsRoot(t *testing.T) {
	m := fstest.MapFS{
		"go.mod": mf("module example.com/lib\n"), "lib.go": mf("package lib\n"), "assets/x.json": mf("{}"),
		"app/go.mod":  mf("module example.com/app\n\nreplace example.com/lib => ../\n"),
		"app/main.go": mf("package main\nimport _ \"example.com/lib\"\n"),
		"tool/go.mod": mf("module example.com/tool\n"), "tool/t.go": mf("package t\n"),
	}
	jobs := []plan.JobRef{{ID: "root", Workdir: "."}, {ID: "app", Workdir: "app"}, {ID: "tool", Workdir: "tool"}, {ID: "assets", Workdir: "assets"}}
	sel, got := selectFor(t, m, jobs, "assets/x.json")
	if sel.Mode != "affected" || !reflect.DeepEqual(got, []string{"app", "assets", "root"}) {
		t.Fatalf("got %v %v", sel, got)
	}
}

func TestFallbackKeepsTheFullRules(t *testing.T) {
	jobs := []plan.JobRef{{ID: "root", Workdir: "."}, {ID: "web", Workdir: "web"}}
	base := plan.SelectionInput{Changed: []string{"Makefile"}, BaseKnown: true, Jobs: jobs}
	if sel, got := Selector(base); sel.Mode != "full" || len(got) != 2 {
		t.Fatalf("Makefile without a graph: %v %v", sel, got)
	}
	rel := plan.SelectionInput{Changed: []string{"x.go"}, BaseKnown: true, Release: true, Jobs: jobs}
	if sel, _ := Selector(rel); sel.Mode != "full" {
		t.Fatalf("release without a graph: %v", sel)
	}
	def := plan.SelectionInput{Changed: []string{"x.go"}, BaseKnown: true, DefaultBranch: true, Jobs: jobs}
	if sel, _ := Selector(def); sel.Mode != "full" {
		t.Fatalf("default branch without a graph: %v", sel)
	}
}

func TestPnpmReviewCases(t *testing.T) {
	cases := []struct {
		name      string
		fs        fstest.MapFS
		edges     []string
		uncertain bool
	}{
		{"brace globs", fstest.MapFS{
			"pnpm-workspace.yaml":     mf("packages: ['packages/{a,b}']\n"),
			"packages/a/package.json": mf(`{"name":"a","dependencies":{"b":"*"}}`),
			"packages/b/package.json": mf(`{"name":"b"}`),
			"packages/c/package.json": mf(`{"name":"c","dependencies":{"b":"*"}}`),
		}, []string{"packages/a->packages/b"}, false},
		{"extglob is not modelled", fstest.MapFS{
			"pnpm-workspace.yaml":     mf("packages: ['packages/!(x)']\n"),
			"packages/a/package.json": mf(`{"name":"a"}`),
		}, nil, true},
		{"workspace alias edges to the real package", fstest.MapFS{
			"pnpm-workspace.yaml": mf("packages: ['p/*']\n"),
			"p/a/package.json":    mf(`{"name":"a","dependencies":{"alias":"workspace:real@*"}}`),
			"p/real/package.json": mf(`{"name":"real"}`),
		}, []string{"p/a->p/real"}, false},
		{"scoped workspace specifier", fstest.MapFS{
			"pnpm-workspace.yaml": mf("packages: ['p/*']\n"),
			"p/a/package.json":    mf(`{"name":"a","dependencies":{"x":"workspace:@s/real@^1"}}`),
			"p/real/package.json": mf(`{"name":"@s/real"}`),
		}, []string{"p/a->p/real"}, false},
		{"duplicate names edge to every holder", fstest.MapFS{
			"pnpm-workspace.yaml": mf("packages: ['p/*']\n"),
			"p/a/package.json":    mf(`{"name":"x"}`),
			"p/b/package.json":    mf(`{"name":"x"}`),
			"p/c/package.json":    mf(`{"name":"c","dependencies":{"x":"workspace:*"}}`),
		}, []string{"p/c->p/a", "p/c->p/b"}, false},
		{"escaping link path is uncertain", fstest.MapFS{
			"pnpm-workspace.yaml": mf("packages: ['apps/*']\n"),
			"apps/a/package.json": mf(`{"name":"a","dependencies":{"ghost":"link:../../../missing"}}`),
		}, nil, true},
		{"link to a tarball is ignored", fstest.MapFS{
			"a/package.json": mf(`{"name":"a","dependencies":{"t":"file:../vendor/t.tgz"}}`),
		}, nil, false},
		{"nested workspace package under build/", fstest.MapFS{
			"pnpm-workspace.yaml":  mf("packages: ['**']\n"),
			"build/a/package.json": mf(`{"name":"a"}`),
			"apps/b/package.json":  mf(`{"name":"b","dependencies":{"a":"*"}}`),
		}, []string{"apps/b->build/a"}, false},
		{"non-string dependency value is uncertain", fstest.MapFS{
			"a/package.json": mf(`{"name":"a","dependencies":{"x":1}}`),
		}, nil, true},
		{"testdata manifests are ignored", fstest.MapFS{
			"a/package.json":                 mf(`{"name":"a"}`),
			"a/testdata/broken/package.json": mf(`{`),
		}, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := buildFS(t, c.fs)
			if got := edgesOf(g); !reflect.DeepEqual(got, c.edges) {
				t.Errorf("edges = %v, want %v", got, c.edges)
			}
			if (len(g.reasons) > 0) != c.uncertain {
				t.Errorf("uncertain = %v (%v), want %v", len(g.reasons) > 0, g.reasons, c.uncertain)
			}
		})
	}
}

func TestOpaqueAndGeneratorZones(t *testing.T) {
	m := fstest.MapFS{
		"go.mod": mf("module m\n"), "g/g.go": mf("package g\n"),
		"rust/a/Cargo.toml": mf("[package]\n"), "rust/b/Cargo.toml": mf("[package]\n"),
		"lib/sqlc.yaml": mf("version: 2\n"), "lib/queries.sql": mf("select 1;"), "lib/db.go": mf("package lib\n"),
	}
	jobs := []plan.JobRef{{ID: "go", Workdir: "g"}, {ID: "a", Workdir: "rust/a"}, {ID: "b", Workdir: "rust/b"}, {ID: "lib", Workdir: "lib"}}
	for _, p := range []string{"rust/a/src/lib.rs", "lib/queries.sql"} {
		if sel, _ := selectFor(t, m, jobs, p); sel.Mode != "full" {
			t.Errorf("%s: %v, want full", p, sel)
		}
	}
	if sel, got := selectFor(t, m, jobs, "g/g.go"); sel.Mode != "affected" || !reflect.DeepEqual(got, []string{"go"}) {
		t.Errorf("g/g.go: %v %v", sel, got)
	}
	single := fstest.MapFS{"go.mod": mf("module m\n"), "g/g.go": mf("package g\n"), "c/Cargo.toml": mf("x"), "c/src/lib.rs": mf("x")}
	if g := buildFS(t, single); len(g.opaque) != 0 {
		t.Errorf("one manifest must not be opaque: %v", g.opaque)
	}
}

func TestQuotedAndBlockReplaceTargets(t *testing.T) {
	mf1 := parseModFile("app", "module a\n\nreplace (\n\tx.org/up v1.0.0 => \"../fork\"\n\tx.org/y => ./y // note\n\tx.org/z => x.org/z2 v1.0.0\n)\n")
	if !reflect.DeepEqual(mf1.replaces, []goModule{{"fork", "x.org/up"}, {"app/y", "x.org/y"}}) {
		t.Fatalf("replaces = %v", mf1.replaces)
	}
}

func TestDuplicateNameEdgesAreStable(t *testing.T) {
	m := fstest.MapFS{
		"pnpm-workspace.yaml": mf("packages: ['p/*']\n"),
		"p/a/package.json":    mf(`{"name":"x"}`), "p/b/package.json": mf(`{"name":"x"}`),
		"p/c/package.json": mf(`{"name":"c","dependencies":{"x":"*"}}`),
	}
	first := edgesOf(buildFS(t, m))
	for i := 0; i < 20; i++ {
		if got := edgesOf(buildFS(t, m)); !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d: %v != %v", i, got, first)
		}
	}
}

func TestSoftIgnoredDirsAreWalked(t *testing.T) {
	m := fstest.MapFS{"build/x/package.json": mf(`{"name":"x"}`), "dist/out.js": mf("x"), "target/debug/a": mf("x"), "Cargo.toml": mf("x")}
	tr, err := walk(m)
	if err != nil {
		t.Fatal(err)
	}
	if !tr.set["build/x/package.json"] || !tr.set["dist/out.js"] || tr.set["target/debug/a"] {
		t.Fatalf("files = %v", tr.files)
	}
}

func TestUnderscoreModuleRootIsImportable(t *testing.T) {
	m := fstest.MapFS{
		"app/go.mod":           mf("module example.com/app\n\nreplace example.com/sdk => ../_libs/sdk\n"),
		"app/main.go":          mf("package main\nimport _ \"example.com/sdk/out\"\n"),
		"_libs/sdk/go.mod":     mf("module example.com/sdk\n"),
		"_libs/sdk/out/o.go":   mf("package out\n"),
		"_libs/sdk/_skip/s.go": mf("package skip\n"),
		"_hidden/p.go":         mf("package p\n"),
	}
	g := buildFS(t, m)
	if got, want := edgesOf(g), []string{"app->_libs/sdk/out"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("edges = %v, want %v (reasons %v)", got, want, g.reasons)
	}
	for _, d := range []string{"_libs/sdk/_skip", "_hidden"} {
		if _, ok := g.eco[d]; ok {
			t.Errorf("%s must not be a package", d)
		}
	}
}

func TestGoWorkMemberReplacesAndQuotedPaths(t *testing.T) {
	m := fstest.MapFS{
		"go.work":           mf("use (\n\t./a\n\t./b\n\t\"./shared lib\"\n)\n"),
		"a/go.mod":          mf("module example.com/a\n"),
		"a/a.go":            mf("package a\nimport _ \"example.com/lib\"\nimport _ \"example.com/shared\"\n"),
		"b/go.mod":          mf("module example.com/b\n\nreplace example.com/lib => ../lib\n"),
		"b/b.go":            mf("package b\n"),
		"lib/go.mod":        mf("module example.com/lib\n"),
		"lib/x.go":          mf("package lib\n"),
		"shared lib/go.mod": mf("module example.com/shared\n"),
		"shared lib/s.go":   mf("package s\n"),
	}
	g := buildFS(t, m)
	want := []string{"a->lib", "a->shared lib"}
	if got := edgesOf(g); !reflect.DeepEqual(got, want) {
		t.Fatalf("edges = %v, want %v (reasons %v)", got, want, g.reasons)
	}
}

func TestUnmodelledReplacePathsAreUncertain(t *testing.T) {
	for _, to := range []string{`/abs/lib`, `..\lib`, `C:\lib`} {
		m := fstest.MapFS{"go.mod": mf("module m\n\nreplace example.com/lib => " + to + "\n"), "a.go": mf("package m\n")}
		if g := buildFS(t, m); len(g.reasons) == 0 {
			t.Errorf("replace => %s must be uncertain", to)
		}
	}
}

func TestRootToolConfigIsUnmapped(t *testing.T) {
	m := fstest.MapFS{
		"package.json": mf(`{"name":"root","packageManager":"pnpm@9"}`), "pnpm-workspace.yaml": mf("packages: ['web/*']\n"),
		"web/a/package.json": mf(`{"name":"a"}`), "eslint.config.js": mf("export default []\n"), "web/tsconfig.base.json": mf("{}"),
	}
	jobs := []plan.JobRef{{ID: "lint", Workdir: "."}, {ID: "web", Workdir: "web"}, {ID: "a.test", Workdir: "web/a"}, {ID: "b.test", Workdir: "web/b"}}
	if sel, _ := selectFor(t, m, jobs, "eslint.config.js"); sel.Mode != "full" {
		t.Errorf("eslint.config.js: %v, want full", sel)
	}
	sel, got := selectFor(t, m, jobs, "web/tsconfig.base.json") // not owned by a package: every job under web
	if sel.Mode != "affected" || !reflect.DeepEqual(got, []string{"a.test", "b.test", "lint", "web"}) {
		t.Errorf("web/tsconfig.base.json: %v %v", sel, got)
	}
}

func TestRootGoPackageSeedsForEverySubtree(t *testing.T) {
	m := fstest.MapFS{
		"go.mod": mf("module example.com/lib\n"), "lib.go": mf("package lib\n"),
		"assets/assets.go": mf("package assets\n"), "assets/x.json": mf("{}"),
		"app/go.mod":  mf("module example.com/app\n\nreplace example.com/lib => ../\n"),
		"app/main.go": mf("package main\nimport _ \"example.com/lib\"\n"),
	}
	jobs := []plan.JobRef{{ID: "root", Workdir: "."}, {ID: "app", Workdir: "app"}}
	if sel, got := selectFor(t, m, jobs, "assets/x.json"); sel.Mode != "affected" || !reflect.DeepEqual(got, []string{"app", "root"}) {
		t.Fatalf("got %v %v", sel, got)
	}
}

func TestDependencySectionsAndPathSpecs(t *testing.T) {
	m := fstest.MapFS{
		"pnpm-workspace.yaml":    mf("packages: ['p/*', 'p/app/lib']\n"),
		"p/real/package.json":    mf(`{"name":"real"}`),
		"p/a/package.json":       mf(`{"name":"a","dependencies":{"alias":"npm:external@1"},"optionalDependencies":{"alias":"workspace:real@*"}}`),
		"p/app/package.json":     mf(`{"name":"app","dependencies":{"lib":"file:lib"}}`),
		"p/app/lib/package.json": mf(`{"name":"lib2"}`),
		"p/client/package.json":  mf(`{"name":"client","dependencies":{"app":"*"}}`),
	}
	g := buildFS(t, m)
	want := []string{"p/a->p/real", "p/app->p/app/lib", "p/client->p/app"}
	if got := edgesOf(g); !reflect.DeepEqual(got, want) {
		t.Fatalf("edges = %v, want %v (reasons %v)", got, want, g.reasons)
	}
}

func TestBraceRangesAndBounds(t *testing.T) {
	for _, c := range []struct {
		pat, p string
		want   bool
	}{
		{"packages/pkg{1..3}", "packages/pkg2", true}, {"packages/pkg{1..3}", "packages/pkg4", false},
		{"x/{a..c}", "x/b", true}, {"x/{a,b/{c,d}}", "x/b/d", true}, {"x/{a", "x/{a", true},
	} {
		if got := matchGlob(c.pat, c.p); got != c.want {
			t.Errorf("matchGlob(%q, %q) = %v", c.pat, c.p, got)
		}
	}
	wide := ""
	for i := 0; i < 32; i++ {
		wide += "{a,b}"
	}
	if alts, ok := expandBraces(wide); ok || len(alts) > maxBraceAlts {
		t.Fatalf("a 2^32 pattern expanded to %d alternatives (ok=%v)", len(alts), ok)
	}
	m := fstest.MapFS{"pnpm-workspace.yaml": mf("packages: ['" + wide + "']\n"), "a/package.json": mf(`{"name":"a"}`)}
	if g := buildFS(t, m); len(g.reasons) == 0 {
		t.Fatal("an unboundedly wide glob must be uncertain")
	}
}

func TestNonCertainFactsInTestdataAreIgnored(t *testing.T) {
	m := fstest.MapFS{"go.mod": mf("module m\n"), "a/a.go": mf("package a\n")}
	g := buildFS(t, m)
	facts := append(g.Facts(),
		detect.Fact{Kind: "service", Name: "unknown", Path: "k8s/testdata/compose.yml", Confidence: detect.Unknown})
	in := plan.SelectionInput{Changed: []string{"a/a.go"}, BaseKnown: true, Jobs: []plan.JobRef{{ID: "j", Workdir: "."}}, Facts: facts}
	if sel, _ := Selector(in); sel.Mode != "affected" {
		t.Fatalf("testdata fact forced %v", sel)
	}
	in.Facts = append(g.Facts(), detect.Fact{Kind: "service", Name: "unknown", Path: "k8s/compose.yml", Confidence: detect.Unknown})
	if sel, _ := Selector(in); sel.Mode != "full" {
		t.Fatalf("a real non-certain fact must force full, got %v", sel)
	}
}

func TestHiddenDirManifestsAndOpaqueOrder(t *testing.T) {
	m := fstest.MapFS{"a/package.json": mf(`{"name":"a"}`), ".next/package.json": mf(`{`)}
	if g := buildFS(t, m); len(g.reasons) != 0 {
		t.Fatalf("a hidden directory manifest must be ignored: %v", g.reasons)
	}
	o := fstest.MapFS{"a/Cargo.toml": mf("x"), "b/Cargo.toml": mf("x"), "a/pyproject.toml": mf("x"), "b/pyproject.toml": mf("x")}
	first := buildFS(t, o).opaque
	for i := 0; i < 20; i++ {
		if got := buildFS(t, o).opaque; !reflect.DeepEqual(got, first) {
			t.Fatalf("opaque map changed between builds: %v vs %v", got, first)
		}
	}
}

func TestThirdRoundCases(t *testing.T) {
	t.Run("brace range past the cap is uncertain", func(t *testing.T) {
		m := fstest.MapFS{"pnpm-workspace.yaml": mf("packages: ['apps/*', 'packages/{1..1025}']\n"), "apps/app/package.json": mf(`{"name":"app"}`)}
		if g := buildFS(t, m); len(g.reasons) == 0 {
			t.Fatal("a too-wide range must be uncertain")
		}
	})
	t.Run("huge integer range does not overflow or allocate", func(t *testing.T) {
		if _, ok := expandBraces("x/{9223372036854775807..9223372036854775807}"); ok {
			t.Fatal("a range outside the bounds must not expand")
		}
		if got, ok := expandBraces("x/{3..1}"); !ok || !reflect.DeepEqual(got, []string{"x/3", "x/2", "x/1"}) {
			t.Fatalf("descending range = %v %v", got, ok)
		}
	})
	t.Run("nested npm workspaces are uncertain", func(t *testing.T) {
		m := fstest.MapFS{"frontend/package.json": mf(`{"name":"f","workspaces":["packages/*"]}`), "frontend/packages/lib/package.json": mf(`{"name":"lib"}`)}
		if g := buildFS(t, m); len(g.reasons) == 0 {
			t.Fatal("nested workspaces without pnpm-workspace.yaml must be uncertain")
		}
	})
	t.Run("escaped module path", func(t *testing.T) {
		m := fstest.MapFS{
			"go.work":    mf("use (\n\t./app\n\t./lib\n)\n"),
			"lib/go.mod": mf("module \"example.com/\\u006cib\"\n"), "lib/l.go": mf("package lib\n"),
			"app/go.mod": mf("module example.com/app\n"), "app/a.go": mf("package app\nimport _ \"example.com/lib\"\n"),
		}
		if got, want := edgesOf(buildFS(t, m)), []string{"app->lib"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("edges = %v, want %v", got, want)
		}
		if g := buildFS(t, fstest.MapFS{"go.mod": mf("module \"m\n"), "a.go": mf("package m\n")}); len(g.reasons) == 0 {
			t.Fatal("an unterminated quote must be uncertain")
		}
	})
	t.Run("a Go package embeds files under nested manifests", func(t *testing.T) {
		m := fstest.MapFS{
			"go.work":    mf("use (\n\t./lib\n\t./app\n)\n"),
			"lib/go.mod": mf("module example.com/lib\n"), "lib/lib.go": mf("package lib\n"),
			"lib/assets/package.json": mf(`{"name":"assets"}`), "lib/assets/a.txt": mf("x"),
			"app/go.mod": mf("module example.com/app\n"), "app/a.go": mf("package app\nimport _ \"example.com/lib\"\n"),
		}
		jobs := []plan.JobRef{{ID: "lib", Workdir: "lib"}, {ID: "app", Workdir: "app"}}
		if sel, got := selectFor(t, m, jobs, "lib/assets/a.txt"); sel.Mode != "affected" || !reflect.DeepEqual(got, []string{"app", "lib"}) {
			t.Fatalf("got %v %v", sel, got)
		}
	})
	t.Run("vendored code forces full", func(t *testing.T) {
		m := fstest.MapFS{"app/cmd/main.go": mf("package main\n"), "app/vendor/example.com/dep/d.go": mf("package dep\n")}
		jobs := []plan.JobRef{{ID: "app", Workdir: "app"}, {ID: "cmd", Workdir: "app/cmd"}}
		if sel, _ := selectFor(t, m, jobs, "app/vendor/example.com/dep/d.go"); sel.Mode != "full" {
			t.Fatalf("got %v", sel)
		}
	})
}

func TestFourthRoundCases(t *testing.T) {
	t.Run("replace( and use( without a space", func(t *testing.T) {
		m := fstest.MapFS{
			"a/go.mod": mf("module example.com/a\nreplace(\n\texample.com/b => ../b\n)\n"), "a/main.go": mf("package main\nimport _ \"example.com/b\"\n"),
			"b/go.mod": mf("module example.com/b\n"), "b/b.go": mf("package b\n"),
		}
		if got, want := edgesOf(buildFS(t, m)), []string{"a->b"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("replace(: edges = %v, want %v", got, want)
		}
		w := fstest.MapFS{
			"go.work": mf("use(\n\t./a\n\t./b\n)\n"), "a/go.mod": mf("module example.com/a\n"), "a/main.go": mf("package main\nimport _ \"example.com/b\"\n"),
			"b/go.mod": mf("module example.com/b\n"), "b/b.go": mf("package b\n"),
		}
		if got, want := edgesOf(buildFS(t, w)), []string{"a->b"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("use(: edges = %v, want %v", got, want)
		}
	})
	t.Run("workspace spec outside every pnpm workspace", func(t *testing.T) {
		m := fstest.MapFS{"apps/web/package.json": mf(`{"name":"web","dependencies":{"ui":"workspace:*"}}`), "packages/ui/package.json": mf(`{"name":"ui"}`)}
		if g := buildFS(t, m); len(g.reasons) == 0 {
			t.Fatal("a workspace: dependency with no workspace must be uncertain")
		}
	})
	t.Run("a submodule bump names the package directory", func(t *testing.T) {
		m := fstest.MapFS{
			"app/go.mod": mf("module example.com/app\n\nreplace example.com/sub => ../third_party/sub\n"), "app/main.go": mf("package main\nimport _ \"example.com/sub\"\n"),
			"third_party/sub/go.mod": mf("module example.com/sub\n"), "third_party/sub/s.go": mf("package sub\n"),
		}
		jobs := []plan.JobRef{{ID: "app", Workdir: "app"}, {ID: "sub", Workdir: "third_party/sub"}}
		if sel, got := selectFor(t, m, jobs, "third_party/sub"); sel.Mode != "affected" || !reflect.DeepEqual(got, []string{"app", "sub"}) {
			t.Fatalf("gitlink bump: %v %v", sel, got)
		}
		if sel, _ := selectFor(t, m, jobs, "third_party"); sel.Mode != "full" {
			t.Fatalf("a directory above known packages must be full, got %v", sel)
		}
	})
	t.Run("a Go package embeds a child frontend", func(t *testing.T) {
		m := fstest.MapFS{
			"go.mod": mf("module example.com/m\n"), "ui/ui.go": mf("package ui\n"), "ui/frontend/package.json": mf(`{"name":"fe"}`), "ui/frontend/dist/index.html": mf("x"),
			"cmd/server/main.go": mf("package main\nimport _ \"example.com/m/ui\"\n"),
		}
		jobs := []plan.JobRef{{ID: "server", Workdir: "cmd/server"}, {ID: "fe", Workdir: "ui/frontend"}}
		if sel, got := selectFor(t, m, jobs, "ui/frontend/dist/index.html"); sel.Mode != "affected" || !reflect.DeepEqual(got, []string{"fe", "server"}) {
			t.Fatalf("got %v %v", sel, got)
		}
	})
	t.Run("project files of other ecosystems are opaque", func(t *testing.T) {
		m := fstest.MapFS{"src/Lib/Lib.csproj": mf("<Project/>"), "src/App/App.csproj": mf("<Project/>"), "src/Lib/Lib.cs": mf("x")}
		jobs := []plan.JobRef{{ID: "lib", Workdir: "src/Lib"}, {ID: "app", Workdir: "src/App"}}
		if sel, _ := selectFor(t, m, jobs, "src/Lib/Lib.cs"); sel.Mode != "full" {
			t.Fatalf("got %v", sel)
		}
	})
	t.Run("many ** do not blow up", func(t *testing.T) {
		done := make(chan bool, 1)
		go func() {
			done <- !matchGlob(strings.Repeat("**/*/", 9)+"zz", strings.Repeat("a/", 40)+"b")
		}()
		select {
		case ok := <-done:
			if !ok {
				t.Fatal("pattern must not match")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("matchGlob did not finish in 5s")
		}
	})
	t.Run("non-regular files are never read", func(t *testing.T) {
		m := fstest.MapFS{"go.mod": mf("module m\n"), "fifo.go": &fstest.MapFile{Mode: fs.ModeNamedPipe}, "a.go": mf("package m\n")}
		tr, err := walk(m)
		if err != nil {
			t.Fatal(err)
		}
		if tr.set["fifo.go"] || !tr.set["a.go"] {
			t.Fatalf("files = %v", tr.files)
		}
	})
	t.Run("reasons are capped", func(t *testing.T) {
		m := fstest.MapFS{"go.mod": mf("module m\n")}
		for i := 0; i < 500; i++ {
			m["p"+strconv.Itoa(i)+"/x.go"] = mf("package x\nimport (\n")
		}
		if g := buildFS(t, m); len(g.reasons) != maxReasons {
			t.Fatalf("reasons = %d, want %d", len(g.reasons), maxReasons)
		}
	})
}

func TestSurvivingMutantsPinned(t *testing.T) {
	t.Run("go.work replace is read", func(t *testing.T) {
		m := fstest.MapFS{
			"go.work":  mf("use ./a\n\nreplace example.com/lib => ./lib\n"),
			"a/go.mod": mf("module example.com/a\n"), "a/a.go": mf("package a\nimport _ \"example.com/lib\"\n"),
			"lib/go.mod": mf("module example.com/lib\n"), "lib/l.go": mf("package lib\n"),
		}
		if got, want := edgesOf(buildFS(t, m)), []string{"a->lib"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("edges = %v, want %v", got, want)
		}
	})
	t.Run("a nested build script forces full", func(t *testing.T) {
		m := fstest.MapFS{"go.mod": mf("module m\n"), "app/a.go": mf("package app\n"), "app/scripts/build.sh": mf("#!/bin/sh\n")}
		jobs := []plan.JobRef{{ID: "app", Workdir: "app"}, {ID: "other", Workdir: "other"}}
		if sel, _ := selectFor(t, m, jobs, "app/scripts/build.sh"); sel.Mode != "full" {
			t.Fatalf("got %v", sel)
		}
	})
	t.Run("no job left means full", func(t *testing.T) {
		m := fstest.MapFS{"go.mod": mf("module m\n"), "g/g.go": mf("package g\n")}
		if sel, _ := selectFor(t, m, []plan.JobRef{{ID: "other", Workdir: "other"}}, "g/g.go"); sel.Mode != "full" {
			t.Fatalf("got %v", sel)
		}
	})
	t.Run("a pkggraph fact of an unknown shape is uncertain", func(t *testing.T) {
		g := buildFS(t, fstest.MapFS{"go.mod": mf("module m\n"), "a/a.go": mf("package a\n")})
		facts := append(g.Facts(), detect.Fact{Kind: FactKind, Name: "mystery", Path: "a", Confidence: detect.Certain})
		in := plan.SelectionInput{Changed: []string{"a/a.go"}, BaseKnown: true, Jobs: []plan.JobRef{{ID: "j", Workdir: "a"}}, Facts: facts}
		if sel, _ := Selector(in); sel.Mode != "full" {
			t.Fatalf("got %v", sel)
		}
	})
}
