package affected

// Regression tests for the review rounds of P7-SCHED-13 (each case is a
// concrete input that once selected fewer jobs than a correct run needs).

import (
	"reflect"
	"testing"
	"testing/fstest"

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
