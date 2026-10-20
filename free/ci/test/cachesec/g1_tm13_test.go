//go:build cachesec

package cachesec

// Purpose: TM-13 cases on the local carrier (writes by a stale attempt): a commit after the attempt
// ended, by an attempt that failed, by one still running, and a replay of a commit. C13-LOC-01
// (older epoch) is the sample in c13_local_test.go.
// Inputs: a real store and cas (newWorld). Outputs: registered cases C13-CAC-01..05.
// Constraints: production packages only. Each case commits an honest entry first (positive
// control) and reports as Landed only the entry bytes the attack added.

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

func init() {
	// The attempt finished (passed) after its first save; a later commit of another planned key is refused.
	register("C13-CAC-01", func(t *testing.T) Observed {
		w := newWorld(t)
		ctl, atk := w.Key(g1PR1, "ctl"), w.Key(g1PR1, "atk")
		fin := w.Attempt("e1", g1PR1, []string{ctl, atk}, true)
		if err := w.g1Commit("e1", fin, g1PR1, ctl, "honest bytes"); err != nil { // positive control
			t.Fatalf("control: %v", err)
		}
		w.g1Transition("e1", "finalizing", "passed", fin)
		base := w.EntryBytes()
		got := refusal(t, w.g1Commit("e1", fin, g1PR1, atk, "late bytes"), g1ReasonE711(t))
		got.Audit = w.AuditOf("cache.commit.refused")
		got.Landed = w.EntryBytes() - base
		return got
	})

	// The attempt recorded a failed result: nothing may be saved.
	register("C13-CAC-02", func(t *testing.T) Observed {
		w := newWorld(t)
		w.g1Honest("ok1", g1PR1, "ctl") // control on the same fixture: a passing attempt saves
		base := w.EntryBytes()
		key := w.Key(g1PR1, "failed")
		fin := w.Attempt("bad1", g1PR1, []string{key}, false)
		got := refusal(t, w.g1Commit("bad1", fin, g1PR1, key, "bytes of a failed attempt"), g1ReasonE711(t))
		got.Audit = w.AuditOf("cache.commit.refused")
		got.Landed = w.EntryBytes() - base
		return got
	})

	// The attempt ended failed (a terminal state) after it recorded a pass.
	register("C13-CAC-03", func(t *testing.T) Observed {
		w := newWorld(t)
		w.g1Honest("ok1", g1PR1, "ctl")
		base := w.EntryBytes()
		key := w.Key(g1PR1, "ended")
		fin := w.Attempt("end1", g1PR1, []string{key}, true)
		w.g1Transition("end1", "finalizing", "failed", fin)
		got := refusal(t, w.g1Commit("end1", fin, g1PR1, key, "bytes after the end"), g1ReasonE711(t))
		got.Audit = w.AuditOf("cache.commit.refused")
		got.Landed = w.EntryBytes() - base
		return got
	})

	// The attempt is still running (planned, not finalizing): it may restore, never save.
	register("C13-CAC-04", func(t *testing.T) Observed {
		w := newWorld(t)
		w.g1Honest("ok1", g1PR1, "ctl")
		base := w.EntryBytes()
		key := w.Key(g1PR1, "running")
		ctx, s := w.ctx, w.St
		for i, err := range []error{
			s.CreatePipeline(ctx, store.PipelineRow{ID: "prun", Revision: "rev", Project: g1PR1.Project, Trigger: "local", SourceTrust: g1PR1.Trust, PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i"}),
			s.CreateJob(ctx, store.JobRow{ID: "jrun", PipelineID: "prun", Name: "run", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}),
			s.CreateAttempt(ctx, store.AttemptRow{ID: "run1", JobID: "jrun", N: 1}, ""),
			storetest.Admit(ctx, s, "run1"),
			s.Transition(ctx, "run1", "leased", "running", store.TransitionDetail{Epoch: 1}),
			w.Es.RecordPlan(ctx, "run1", 2, g1PR1, []string{key}),
		} {
			if err != nil {
				t.Fatalf("running attempt, step %d: %v", i, err)
			}
		}
		got := refusal(t, w.g1Commit("run1", 2, g1PR1, key, "bytes of a running attempt"), g1ReasonE711(t))
		got.Audit = w.AuditOf("cache.commit.refused")
		got.Landed = w.EntryBytes() - base
		return got
	})

	// A replay of a finished commit (same key, other bytes): the plan was consumed.
	register("C13-CAC-05", func(t *testing.T) Observed {
		w := newWorld(t)
		key := w.Key(g1PR1, "once")
		fin := w.Attempt("r1", g1PR1, []string{key}, true)
		if err := w.g1Commit("r1", fin, g1PR1, key, "first bytes"); err != nil { // positive control
			t.Fatalf("control: %v", err)
		}
		base := w.EntryBytes()
		got := refusal(t, w.g1Commit("r1", fin, g1PR1, key, "replayed and different bytes"), g1ReasonE711(t))
		got.Audit = w.AuditOf("cache.commit.refused")
		got.Landed = w.EntryBytes() - base
		return got
	})
}
