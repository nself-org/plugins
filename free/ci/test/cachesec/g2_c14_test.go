//go:build cachesec

package cachesec

// Purpose: TM-14 cases C14-ART-03..20 on the local artifact store: crafted artifact trees and
// declarations (E683 path refusals), size caps at collection time (E712) and `artifacts get`
// destinations (E683). Production refuses each and stores nothing, but appends no audit row: the
// rows say `audit: none, gap: D-1450` and each case asserts that no audit row appeared.
// Inputs: g2World. Outputs: registered cases of matrix.yaml.
// Constraints: production packages only; an honest control of the same operation runs first on the
// same fixture; Landed is the new blob and record bytes (collect) or new bytes under the cwd (get).
// Audit snapshot: records.Collector, records.Reader, records.Normalize and authz.ArtifactPath are
// pure or sinkless: they take no audit hook, so no audit row CAN exist for these refusals today and
// `audit: none` holds by construction. The gap (D-1450) is the missing hook. The cases still compare
// the world sink before and after (g.Mark/g.AuditSince), so the day a caller wires the world sink
// into the call the snapshot sees the row; a row appended elsewhere is not detected by these cases.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func g2Collect(g *g2World, path string) error {
	ds, err := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{{Path: path}}}, false, nil)
	if err != nil {
		return err
	}
	_, err = g.Collector().Collect(g.ctx, g2Project, g.Ws, ds)
	return err
}

// g2Outside is a file outside the workdir the attacks point at.
func g2Outside(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(p, []byte("host file"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func g2Must(t *testing.T, errs ...error) {
	t.Helper()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("fixture step %d: %v", i, err)
		}
	}
}

// g2CollectCase builds a case: honest collection of ok.txt, then the attack path from setup.
func g2CollectCase(setup func(t *testing.T, g *g2World) string) func(*testing.T) Observed {
	return func(t *testing.T) Observed {
		g := newG2World(t)
		g.Write("ok.txt", "fine")
		if err := g2Collect(g, "ok.txt"); err != nil {
			t.Fatalf("control: %v", err)
		}
		path := setup(t, g)
		g.Mark()
		got, _ := g2PathObserved(t, g2Collect(g, path))
		got.Audit, got.Landed = g.AuditSince(), g.LandedSince()
		return got
	}
}

// g2SanitiserCase runs one entry through the sanitiser every collected entry passes (devices and bombs cannot be made unprivileged on disk).
func g2SanitiserCase(honest, evil authz.Entry) func(*testing.T) Observed {
	return func(t *testing.T) Observed {
		g := newG2World(t)
		if _, err := authz.ArtifactPath(honest, authz.Limits{}); err != nil {
			t.Fatalf("control: %v", err)
		}
		g.Mark()
		_, err := authz.ArtifactPath(evil, authz.Limits{})
		got, _ := g2PathObserved(t, err)
		got.Audit, got.Landed = g.AuditSince(), g.LandedSince()
		return got
	}
}

// g2CapCase refuses a collection under a tight policy with E712. The limits are applied to the collector
// BEFORE the honest control, and the very same collector runs the attack: a collector that rejected honest
// input would fail the control.
func g2CapCase(tune func(c *records.Collector), setup func(t *testing.T, g *g2World) string) func(*testing.T) Observed {
	return func(t *testing.T) Observed {
		g := newG2World(t)
		c := g.Collector()
		tune(&c)
		collect := func(path string) error {
			ds, err := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{{Path: path}}}, false, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.Collect(g.ctx, g2Project, g.Ws, ds)
			return err
		}
		g.Write("ok.txt", "fine")
		if err := collect("ok.txt"); err != nil {
			t.Fatalf("control (tuned collector): %v", err)
		}
		path := setup(t, g)
		g.Mark()
		got := refusal(t, collect(path), func(err error) string { return g2RecordsReason(t, err) })
		got.Audit, got.Landed = g.AuditSince(), g.LandedSince()
		return got
	}
}

func g2GetCase(setup func(t *testing.T, cwd string) string) func(*testing.T) Observed {
	return func(t *testing.T) Observed {
		g := newG2World(t)
		g.Write("get.txt", "artifact bytes")
		rec := g.Pass("gt", "get.txt")[0]
		rd := records.Reader{Ledger: g.St, Blobs: g.Blobs}
		v := authz.Viewer{Project: g2Project, Role: authz.RoleMember}
		cwd := t.TempDir()
		if p, _, err := rd.Get(g.ctx, v, rec.ID, "fresh", cwd); err != nil || filepath.Base(p) != "get.txt" {
			t.Fatalf("control get into a fresh directory: %v %v", p, err)
		}
		dest := setup(t, cwd)
		before := g2TreeBytes(cwd)
		g.Mark()
		_, _, err := rd.Get(g.ctx, v, rec.ID, dest, cwd)
		got, _ := g2PathObserved(t, err)
		got.Audit, got.Landed = g.AuditSince(), g.LandedSince()+max(0, g2TreeBytes(cwd)-before)
		return got
	}
}

func init() {
	collect := []struct {
		id    string
		setup func(t *testing.T, g *g2World) string
	}{
		{"C14-ART-03", func(t *testing.T, g *g2World) string {
			g2Must(t, os.Symlink("../../outside", filepath.Join(g.Ws, "escape")))
			return "escape"
		}},
		{"C14-ART-04", func(t *testing.T, g *g2World) string {
			g.Write("d/a.txt", "a")
			g2Must(t, os.Symlink("/etc/passwd", filepath.Join(g.Ws, "d", "abs")))
			return "d"
		}},
		{"C14-ART-05", func(t *testing.T, g *g2World) string {
			g2Must(t, os.Symlink("ok.txt", filepath.Join(g.Ws, "inlink")))
			return "inlink"
		}},
		{"C14-ART-06", func(t *testing.T, g *g2World) string {
			g2Must(t, os.Link(g2Outside(t), filepath.Join(g.Ws, "hard")))
			return "hard"
		}},
		{"C14-ART-07", func(t *testing.T, g *g2World) string {
			g2Must(t, g2Mkfifo(filepath.Join(g.Ws, "pipe")))
			return "pipe"
		}},
		{"C14-ART-08", func(t *testing.T, g *g2World) string {
			g.Write("suid.bin", "#!/bin/sh")
			g2Must(t, os.Chmod(filepath.Join(g.Ws, "suid.bin"), 0o755|fs.ModeSetuid))
			return "suid.bin"
		}},
	}
	for _, c := range collect {
		register(c.id, g2CollectCase(c.setup))
	}
	register("C14-ART-09", g2SanitiserCase(authz.Entry{Name: "dev", Mode: 0, Size: 1}, authz.Entry{Name: "dev", Mode: fs.ModeDevice}))
	register("C14-ART-10", g2SanitiserCase(authz.Entry{Name: "bomb", Size: 1 << 30, Packed: 1 << 28}, authz.Entry{Name: "bomb", Size: 1 << 30, Packed: 1 << 10}))
	for id, p := range map[string]string{"C14-ART-11": "../../etc/passwd", "C14-ART-12": "/etc/passwd", "C14-ART-13": ".git/hooks/pre-commit"} {
		register(id, func(t *testing.T) Observed { // a declaration is judged before the job runs
			g := newG2World(t)
			if _, err := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{{Path: "ok.txt"}}}, false, nil); err != nil {
				t.Fatalf("control: %v", err)
			}
			g.Mark()
			_, err := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{{Path: p}}}, false, nil)
			got, _ := g2PathObserved(t, err)
			got.Audit, got.Landed = g.AuditSince(), g.LandedSince()
			return got
		})
	}
	register("C14-ART-14", g2CapCase(func(c *records.Collector) { c.Policy.MaxFile = 1024 }, func(t *testing.T, g *g2World) string {
		g.Write("huge.bin", strings.Repeat("h", 2048))
		return "huge.bin"
	}))
	register("C14-ART-15", g2CapCase(func(c *records.Collector) { c.Policy.MaxTotal = 1500 }, func(t *testing.T, g *g2World) string {
		g.Write("m/1.bin", strings.Repeat("1", 1000))
		g.Write("m/2.bin", strings.Repeat("2", 1000))
		return "m"
	}))
	register("C14-ART-16", g2CapCase(func(c *records.Collector) { c.MaxRecords = 2 }, func(t *testing.T, g *g2World) string {
		for _, n := range []string{"n/1", "n/2", "n/3"} {
			g.Write(n, "x")
		}
		return "n"
	}))
	register("C14-ART-17", g2GetCase(func(t *testing.T, cwd string) string { return "." }))
	register("C14-ART-18", g2GetCase(func(t *testing.T, cwd string) string {
		g2Must(t, os.MkdirAll(filepath.Join(cwd, "full"), 0o755), os.WriteFile(filepath.Join(cwd, "full", "mine.txt"), []byte("keep"), 0o644))
		return "full"
	}))
	register("C14-ART-19", g2GetCase(func(t *testing.T, cwd string) string {
		g2Must(t, os.MkdirAll(filepath.Join(cwd, ".git", "hooks"), 0o755))
		return ".git/hooks"
	}))
	register("C14-ART-20", g2GetCase(func(t *testing.T, cwd string) string {
		g2Must(t, os.MkdirAll(filepath.Join(cwd, "elsewhere"), 0o755), os.Symlink(filepath.Join(cwd, "elsewhere"), filepath.Join(cwd, "link")))
		return "link"
	}))
}
