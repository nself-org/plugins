package affected

import (
	"os"
	"reflect"
	"sort"
	"testing"
	"testing/fstest"

	"github.com/nself-org/plugins/free/ci/internal/detect"
)

func mf(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

func edgesOf(g *Graph) []string {
	var out []string
	for k := range g.edges {
		out = append(out, k[0]+"->"+k[1])
	}
	sort.Strings(out)
	return out
}

func TestMonorepoGraph(t *testing.T) {
	g, err := Build(os.DirFS("testdata/monorepo"))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.reasons) != 0 {
		t.Fatalf("fixture graph is uncertain: %v", g.reasons)
	}
	want := []string{"app->mid", "mid->leaf", "web/packages/site->web/packages/ui"}
	if got := edgesOf(g); !reflect.DeepEqual(got, want) {
		t.Fatalf("edges = %v, want %v", got, want)
	}
	for _, d := range []string{"leaf", "mid", "app", "solo", "web/packages/ui", "web/packages/site", "web/packages/lonely", "."} {
		if _, ok := g.eco[d]; !ok {
			t.Errorf("missing package %s", d)
		}
	}
}

func TestFactsRoundTrip(t *testing.T) {
	g, err := Build(os.DirFS("testdata/monorepo"))
	if err != nil {
		t.Fatal(err)
	}
	back, present, certain := FromFacts(g.Facts())
	if !present || !certain {
		t.Fatalf("present=%v certain=%v", present, certain)
	}
	if !reflect.DeepEqual(edgesOf(back), edgesOf(g)) || !reflect.DeepEqual(back.eco, g.eco) {
		t.Fatal("graph changed through its facts")
	}
	if _, present, _ := FromFacts([]detect.Fact{{Kind: "stack", Name: "go", Path: "go.mod", Confidence: detect.Certain}}); present {
		t.Fatal("non-pkggraph facts counted")
	}
	if _, _, certain := FromFacts([]detect.Fact{{Kind: FactKind, Name: "mystery", Path: "x", Confidence: detect.Certain}}); certain {
		t.Fatal("an unknown fact shape must make the graph uncertain")
	}
}

func TestGoGraphCases(t *testing.T) {
	cases := []struct {
		name      string
		fs        fstest.MapFS
		edges     []string
		uncertain bool
	}{
		{"tests and external test packages count", fstest.MapFS{
			"go.mod":       mf("module m\n"),
			"a/a.go":       mf("package a\n"),
			"b/b_test.go":  mf("package b_test\nimport _ \"m/a\"\n"),
			"c/c.go":       mf("package c\nimport (\n\t\"fmt\"\n\t_ \"m/b\"\n\t_ \"golang.org/x/text\"\n)\n"),
			"b/b.go":       mf("package b\n"),
			"c/c_tag.go":   mf("//go:build ignore\n\npackage c\nimport _ \"m/a\"\n"),
			"c/testdata/x": mf("not go"),
		}, []string{"b->a", "c->a", "c->b"}, false},
		{"nested module wins by longest path", fstest.MapFS{
			"go.mod":     mf("module m\n"),
			"x/x.go":     mf("package x\nimport _ \"m/sub/y\"\n"),
			"sub/go.mod": mf("module m/sub\n"),
			"sub/y/y.go": mf("package y\n"),
		}, []string{"x->sub/y"}, false},
		{"testdata packages are not packages", fstest.MapFS{
			"go.mod":              mf("module m\n"),
			"a/a.go":              mf("package a\n"),
			"a/testdata/fix/f.go": mf("package fix\nimport _ \"m/missing\"\n"),
		}, nil, false},
		{"in-repo import with no package is uncertain", fstest.MapFS{
			"go.mod": mf("module m\n"),
			"a/a.go": mf("package a\nimport _ \"m/gone\"\n"),
		}, nil, true},
		{"unparsable file is uncertain", fstest.MapFS{
			"go.mod": mf("module m\n"),
			"a/a.go": mf("package a\nimport (\n"),
		}, nil, true},
		{"go.mod without module is uncertain", fstest.MapFS{
			"go.mod": mf("go 1.22\n"),
			"a/a.go": mf("package a\n"),
		}, nil, true},
		{"package named build is still a package", fstest.MapFS{
			"go.mod":         mf("module m\n"),
			"internal/b.go":  mf("package internal\nimport _ \"m/build\"\n"),
			"build/build.go": mf("package build\n"),
			"dist/out.js":    mf("x"),
		}, []string{"internal->build"}, false},
		{"vendor is skipped", fstest.MapFS{
			"go.mod":             mf("module m\n"),
			"a/a.go":             mf("package a\nimport _ \"example.org/v\"\n"),
			"vendor/v/v.go":      mf("package v\nimport _ \"m/zzz\"\n"),
			"vendor/modules.txt": mf("x"),
		}, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, err := Build(c.fs)
			if err != nil {
				t.Fatal(err)
			}
			if got := edgesOf(g); !reflect.DeepEqual(got, c.edges) {
				t.Errorf("edges = %v, want %v", got, c.edges)
			}
			if (len(g.reasons) > 0) != c.uncertain {
				t.Errorf("uncertain = %v (%v), want %v", len(g.reasons) > 0, g.reasons, c.uncertain)
			}
		})
	}
}

func TestPnpmGraphCases(t *testing.T) {
	cases := []struct {
		name      string
		fs        fstest.MapFS
		edges     []string
		uncertain bool
	}{
		{"workspace and plain version edges", fstest.MapFS{
			"pnpm-workspace.yaml":   mf("packages:\n  - 'pkgs/**'\n"),
			"package.json":          mf(`{"name":"root"}`),
			"pkgs/a/package.json":   mf(`{"name":"a","dependencies":{"b":"workspace:^"}}`),
			"pkgs/b/package.json":   mf(`{"name":"b","devDependencies":{"c":"^1.0.0"}}`),
			"pkgs/c/x/package.json": mf(`{"name":"c"}`),
		}, []string{"pkgs/a->pkgs/b", "pkgs/b->pkgs/c/x"}, false},
		{"unresolvable workspace dependency is uncertain", fstest.MapFS{
			"pnpm-workspace.yaml": mf("packages: ['pkgs/*']\n"),
			"pkgs/a/package.json": mf(`{"name":"a","dependencies":{"ghost":"workspace:*"}}`),
		}, nil, true},
		{"external dependency is no edge", fstest.MapFS{
			"pnpm-workspace.yaml": mf("packages: ['pkgs/*']\n"),
			"pkgs/a/package.json": mf(`{"name":"a","dependencies":{"react":"^19"}}`),
		}, nil, false},
		{"excluded package is outside the workspace", fstest.MapFS{
			"pnpm-workspace.yaml": mf("packages: ['pkgs/*', '!pkgs/b']\n"),
			"pkgs/a/package.json": mf(`{"name":"a","dependencies":{"b":"workspace:*"}}`),
			"pkgs/b/package.json": mf(`{"name":"b"}`),
		}, nil, true},
		{"relative link edge", fstest.MapFS{
			"a/package.json": mf(`{"name":"a","dependencies":{"b":"link:../b"}}`),
			"b/package.json": mf(`{"name":"b"}`),
		}, []string{"a->b"}, false},
		{"npm workspaces without pnpm file are uncertain", fstest.MapFS{
			"package.json":     mf(`{"name":"r","workspaces":["p/*"]}`),
			"p/a/package.json": mf(`{"name":"a"}`),
		}, nil, true},
		{"invalid manifest is uncertain", fstest.MapFS{
			"a/package.json": mf(`{`),
		}, nil, true},
		{"invalid workspace yaml is uncertain", fstest.MapFS{
			"pnpm-workspace.yaml": mf("packages: [unclosed\n"),
			"a/package.json":      mf(`{"name":"a"}`),
		}, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, err := Build(c.fs)
			if err != nil {
				t.Fatal(err)
			}
			if got := edgesOf(g); !reflect.DeepEqual(got, c.edges) {
				t.Errorf("edges = %v, want %v", got, c.edges)
			}
			if (len(g.reasons) > 0) != c.uncertain {
				t.Errorf("uncertain = %v (%v), want %v", len(g.reasons) > 0, g.reasons, c.uncertain)
			}
		})
	}
}

func TestMatchGlob(t *testing.T) {
	for _, c := range []struct {
		pat, p string
		want   bool
	}{
		{"packages/*", "packages/a", true}, {"packages/*", "packages/a/b", false},
		{"packages/**", "packages/a/b", true}, {"packages/**", "packages", true},
		{"**", "a/b/c", true}, {"a/*/c", "a/b/c", true}, {"a/*/c", "a/c", false},
		{"apps/*-web", "apps/x-web", true}, {"apps/*-web", "apps/x-api", false},
		{"**/x", "a/b/x", true}, {"**/x", "x", true},
	} {
		if got := matchGlob(c.pat, c.p); got != c.want {
			t.Errorf("matchGlob(%q, %q) = %v", c.pat, c.p, got)
		}
	}
}

func TestWalkBounds(t *testing.T) {
	m := fstest.MapFS{"go.mod": mf("module m\n")}
	for i := 0; i < maxFiles+5; i++ {
		m["f/"+string(rune('a'+i%26))+"/"+itoa(i)+".txt"] = mf("x")
	}
	g, err := Build(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.reasons) == 0 {
		t.Fatal("a tree over the file cap must be uncertain")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}
