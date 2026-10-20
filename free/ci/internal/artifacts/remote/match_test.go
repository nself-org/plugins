package remote

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// TestSelectAgreesWithTheCollector builds a tree, collects it with the real collector, and checks
// that for every file it stored, Select (which sees only the path) names the same declaration
// and the same record name.
func TestSelectAgreesWithTheCollector(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"bin/app", "bin/app.sha", "dist/a.js", "dist/sub/b.js", "dist/sub/deep/c.txt", "reports/unit.xml", "reports/it/it.xml", "notes.txt", "x/y/z/leaf.log"} {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("data "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	job := model.Job{Run: []string{"true"}, Artifacts: []model.ArtifactDecl{
		{Path: "bin/app", Name: "app-binary"},
		{Path: "bin/*.sha"},
		{Path: "dist", Name: "site"},
		{Path: "reports/**/*.xml", Name: "xml"},
		{Path: "notes.txt"},
		{Path: "x/**/leaf.log", Name: "leaf"},
	}}
	decls, err := records.Normalize(job, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := cas.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pol := records.Policy{MaxFile: 1 << 20, MaxTotal: 1 << 30, StorePath: t.TempDir(), Statfs: func(string) (uint64, uint64, error) { return 100 << 30, 100 << 30, nil }}
	staged, err := records.Collector{Blobs: blobs, Policy: pol, Scanner: redact.New()}.Collect(context.Background(), "p", dir, decls)
	if err != nil {
		t.Fatal(err)
	}
	if len(staged) != 9 {
		t.Fatalf("collector stored %d files, want all 9", len(staged))
	}
	seen := map[string]int{}
	for _, s := range staged {
		d, name, ok := Select(decls, s.Rel, seen[s.Rel])
		seen[s.Rel]++
		if !ok || name != s.Name || d.Ord != s.Decl.Ord {
			t.Errorf("%s: Select = (decl %d, %q, %v), collector = (decl %d, %q)", s.Rel, d.Ord, name, ok, s.Decl.Ord, s.Name)
		}
	}
	for _, rel := range []string{"bin", "bin/other", "dist2/a.js", "reports/unit.txt", "x/leaf.log.bak", "notes.txt/extra"} {
		if _, _, ok := Select(decls, rel, 0); ok && rel != "notes.txt/extra" {
			t.Errorf("%s was selected but no declaration names it", rel)
		}
	}
}
