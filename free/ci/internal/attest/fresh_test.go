package attest_test

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func reasonOf(f model.Freshness) string {
	if f.Reason == nil {
		return ""
	}
	return *f.Reason
}

func TestFresh_CurrentWhenAllInputsMatch(t *testing.T) {
	ev := goodEvidence()
	if f := attest.Fresh(ev, ev.InputDigests); f.State != "current" || f.Reason != nil {
		t.Fatalf("got %+v", f)
	}
}

func TestFresh_ChangeFlipsToStaleNamingTheInput(t *testing.T) {
	rows := []struct {
		name  string
		bend  func(*model.InputDigests)
		named string
	}{
		{"config", func(d *model.InputDigests) { d.Config = strings.Repeat("e", 64) }, "config"},
		{"lockfile changed", func(d *model.InputDigests) { d.Lockfiles[1].SHA256 = strings.Repeat("e", 64) }, "lockfile pnpm-lock.yaml changed"},
		{"lockfile added", func(d *model.InputDigests) {
			d.Lockfiles = append(d.Lockfiles, model.DigestRef{Path: "Cargo.lock", SHA256: "x"})
		}, "lockfile Cargo.lock added"},
		{"lockfile removed", func(d *model.InputDigests) { d.Lockfiles = d.Lockfiles[:1] }, "lockfile pnpm-lock.yaml removed"},
		{"generated changed", func(d *model.InputDigests) { d.Generated[0].SHA256 = strings.Repeat("e", 64) }, "generated gen/api.go changed"},
		{"generated removed", func(d *model.InputDigests) { d.Generated = nil }, "generated gen/api.go removed"},
		{"generated added", func(d *model.InputDigests) {
			d.Generated = append(d.Generated, model.DigestRef{Path: "z.go", SHA256: "x"})
		}, "generated z.go added"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			ev := goodEvidence()
			cur := model.InputDigests{Config: ev.InputDigests.Config,
				Lockfiles: append([]model.DigestRef(nil), ev.InputDigests.Lockfiles...), Generated: append([]model.DigestRef(nil), ev.InputDigests.Generated...)}
			row.bend(&cur)
			f := attest.Fresh(ev, cur)
			if f.State != "stale" || !strings.Contains(reasonOf(f), row.named) {
				t.Fatalf("got %+v (%s), want stale naming %q", f, reasonOf(f), row.named)
			}
		})
	}
}

func TestFresh_UnknownWhenNothingToCompare(t *testing.T) {
	ev := goodEvidence()
	if f := attest.Fresh(ev, model.InputDigests{}); f.State != "unknown" {
		t.Fatalf("no current inputs: %+v", f)
	}
	ev.InputDigests.Config = ""
	if f := attest.Fresh(ev, goodEvidence().InputDigests); f.State != "unknown" {
		t.Fatalf("no bound config: %+v", f)
	}
	ev = goodEvidence()
	ev.InputDigests.Lockfiles = append(ev.InputDigests.Lockfiles, ev.InputDigests.Lockfiles[0])
	if f := attest.Fresh(ev, goodEvidence().InputDigests); f.State != "unknown" {
		t.Fatalf("repeated path: %+v", f)
	}
	ev = goodEvidence()
	ev.InputDigests.Generated[0].Path = ""
	if f := attest.Fresh(ev, goodEvidence().InputDigests); f.State != "unknown" {
		t.Fatalf("empty path: %+v", f)
	}
}

func TestFresh_EmptyDigestNeverMatches(t *testing.T) {
	ev := goodEvidence()
	ev.InputDigests.Lockfiles[0].SHA256 = ""
	cur := goodEvidence().InputDigests
	cur.Lockfiles[0].SHA256 = ""
	if f := attest.Fresh(ev, cur); f.State != "stale" {
		t.Fatalf("empty matched empty: %+v", f)
	}
}

func TestFresh_IgnoresStoredFreshnessAndOrder(t *testing.T) {
	ev := goodEvidence()
	ev.Freshness = model.Freshness{State: "stale", Reason: strp("stored")}
	cur := goodEvidence().InputDigests
	cur.Lockfiles[0], cur.Lockfiles[1] = cur.Lockfiles[1], cur.Lockfiles[0]
	if f := attest.Fresh(ev, cur); f.State != "current" {
		t.Fatalf("stored freshness or order leaked: %+v", f)
	}
}

func TestFresh_FirstChangeIsDeterministic(t *testing.T) {
	ev := goodEvidence()
	cur := goodEvidence().InputDigests
	cur.Config = "other"
	cur.Lockfiles[0].SHA256 = "x"
	if f := attest.Fresh(ev, cur); reasonOf(f) != "config changed" {
		t.Fatalf("config should be named first: %s", reasonOf(f))
	}
	cur.Config = ev.InputDigests.Config
	cur.Generated[0].SHA256 = "y"
	if f := attest.Fresh(ev, cur); reasonOf(f) != "lockfile go.sum changed" {
		t.Fatalf("lockfile should be named before generated: %s", reasonOf(f))
	}
}
