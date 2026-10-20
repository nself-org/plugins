//go:build cachesec

package cachesec

// Purpose: TM-12 on the remote carriers: a remote attempt aims its cache or artifact write at another trust
// class, another PR or another project, and the coordinator-side handler refuses it.
// Inputs: g3rig fixtures with scripted nodes. Outputs: registered cases of matrix.d/g3.yaml.
// Constraints: production handlers only; each refusal has an honest control on a fresh world.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

var g3Job = model.Job{Kind: "test", Run: []string{"true"}}

// g3ControlSave commits an honest cache save for facts f into its own namespace over the carrier.
func g3ControlSave(t *testing.T, carrier string, f entries.Facts, tgz []byte) {
	t.Helper()
	w := newWorld(t)
	r := newG3Rig(t, w, "tctl1", f, g3Plan(g3NS(t, f), "rw", g3Key), g3Job)
	n := &g3node{steps: []g3step{g3Send("cache", tgz), g3CacheRef(g3Key, [2]any{"mod", tgz})}}
	g3Run(t, carrier, r, n, r.Sink())
	if err := r.cache.Finalize(context.Background(), 2*time.Second); err != nil || w.EntryBytes() == 0 {
		t.Fatalf("control (%s): honest save by %+v not admitted: %v", carrier, f, err)
	}
}

// g3AimedSave: attempt facts f is handed a plan whose namespace is aimed at target; the node uploads and refs.
func g3AimedSave(t *testing.T, carrier string, f entries.Facts, target entries.Facts, wantCode, wantAudit string) Observed {
	tgz := g3Tgz(t, map[string]string{"a.txt": "SYNTHETIC poison payload"})
	g3ControlSave(t, carrier, g3Main, tgz)
	w := newWorld(t)
	r := newG3Rig(t, w, "atk1", f, g3Plan(g3NS(t, target), "rw", g3Key), g3Job)
	n := &g3node{steps: []g3step{g3Send("cache", tgz), g3CacheRef(g3Key, [2]any{"mod", tgz})}}
	s := r.Sink()
	g3Run(t, carrier, r, n, s)
	ferr := r.cache.Finalize(context.Background(), 2*time.Second)
	var first error
	for _, e := range append(s.Errs(), ferr) {
		if c, _ := g3Reasons(e); c == wantCode {
			first = e
			break
		}
	}
	got := g3Refused(t, first)
	got.Audit = w.AuditOf(wantAudit)
	got.Landed = r.Landed()
	return got
}

func init() {
	// a fork PR's attempt, handed a plan aimed at main's namespace, cannot write there
	register("C12-SSH-01", func(t *testing.T) Observed {
		return g3AimedSave(t, "ssh", g3Fork1, g3Main, "E711", "cache.remote.refused")
	})
	// PR 1's attempt aimed at PR 2's namespace
	register("C12-AGT-01", func(t *testing.T) Observed {
		return g3AimedSave(t, "agent", g3Fork1, g3Fork2, "E711", "cache.remote.refused")
	})
	// an owner branch job (unprotected) aimed at the protected main namespace: the store recomputes the namespace from the stored facts
	register("C12-SSH-02", func(t *testing.T) Observed {
		branch := entries.Facts{Project: "proj", Trust: "owner", Protection: "unprotected"}
		return g3AimedSave(t, "ssh", branch, g3Main, "E684", "cache.commit.denied")
	})
	// a ref naming a blob that exists only in main's cache realm cannot publish it into the attacker's namespace
	register("C12-COO-01", func(t *testing.T) Observed {
		branch := entries.Facts{Project: "proj", Trust: "owner", Protection: "unprotected"}
		tgz := g3Tgz(t, map[string]string{"a.txt": "SYNTHETIC main-only payload"})
		g3ControlSave(t, "ssh", branch, tgz)
		w := newWorld(t)
		mainNS, err := entries.NamespaceOf(g3Main)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = w.Blobs.Put(w.ctx, cas.CacheRealm(string(mainNS.Canonical())), bytesReader(tgz)); err != nil {
			t.Fatal(err)
		}
		r := newG3Rig(t, w, "atk2", branch, g3Plan(g3NS(t, branch), "rw", g3Key), g3Job)
		// the ssh carrier itself refuses a ref for a blob not received in its session (E656), so the handler is driven
		// directly: the ref names a blob that is "already there" in main's realm only
		s := r.Sink()
		got := g3Refused(t, s.CacheRef(w.ctx, protocol.CacheRef{LeaseID: r.leaseID, Spec: "go", Key: g3Key, Blobs: []protocol.CacheBlob{{Path: "mod", SHA256: g3Sum(tgz), Size: int64(len(tgz))}}}))
		got.Audit = w.AuditOf("cache.remote.refused")
		got.Landed = r.Landed()
		return got
	})
}

var _ = authz.ModeNone
