//go:build integration

package remote

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// outputs is a job's declared tree: one file above two chunks, one small, one duplicate of the small one.
func outputs() map[string][]byte {
	return map[string][]byte{"out/big.bin": pattern(1_300_000), "out/small.txt": []byte("small artifact bytes"), "out/same.txt": []byte("small artifact bytes")}
}

func outputJob() model.Job {
	return model.Job{Kind: "build", Run: []string{"true"}, Artifacts: []model.ArtifactDecl{{Path: "out/big.bin", Name: "big"}, {Path: "out/*.txt"}}}
}

// committed finishes the attempt the way the run does and returns the committed records.
func (r *rig) committed() ([]records.Record, error) {
	r.t.Helper()
	ctx := context.Background()
	state, epoch, err := r.st.AttemptStateEpoch(ctx, "a1")
	if err != nil {
		r.t.Fatal(err)
	}
	if state != "finalizing" {
		r.t.Fatalf("attempt state %s after the result, want finalizing", state)
	}
	return records.CommitAndVerify(ctx, r.st, r.blobs, "a1", epoch, r.pol)
}

// get reads an artifact's bytes through the store, as `ci artifacts get` does.
func (r *rig) get(rec records.Record) []byte {
	r.t.Helper()
	rc, err := r.blobs.Open(cas.ArtifactRealm(rec.Project), rec.SHA256)
	if err != nil {
		r.t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	b, _ := io.ReadAll(rc)
	return b
}

func (r *rig) assertDelivered(want map[string][]byte) {
	r.t.Helper()
	recs, err := r.committed()
	if err != nil {
		r.t.Fatal(err)
	}
	if len(recs) != len(want) {
		r.t.Fatalf("committed %d records, want %d", len(recs), len(want))
	}
	byPath := map[string]string{"out/big.bin": "big", "out/small.txt": "out/small.txt", "out/same.txt": "out/same.txt"}
	for p, b := range want {
		var found bool
		for _, rec := range recs {
			if rec.Name == byPath[p] {
				found = true
				if rec.SHA256 != sum(b) || rec.State != records.StateCommitted || !bytes.Equal(r.get(rec), b) {
					r.t.Fatalf("%s: record %+v does not give back the bytes the job wrote", p, rec)
				}
			}
		}
		if !found {
			r.t.Fatalf("no record for %s (%s)", p, byPath[p])
		}
	}
}

func (r *rig) nothingVisible(files map[string][]byte) {
	r.t.Helper()
	for p, b := range files {
		if r.has(b) {
			r.t.Fatalf("%s is visible in the realm", p)
		}
	}
	staged, _ := r.st.ArtifactStaged(context.Background(), "a1")
	if len(staged) != 0 {
		r.t.Fatalf("%d records staged", len(staged))
	}
}

func TestRemoteArtifactsSSH(t *testing.T) {
	t.Run("delivered", func(t *testing.T) {
		r := newRig(t, outputJob())
		n := &node{t: t, files: outputs(), job: outputJob()}
		if err := r.runSSH(n); err != nil {
			t.Fatal(err)
		}
		r.assertDelivered(outputs())
		checkFrames(t, n.sent())
	})
	t.Run("flipped byte in transit ends the lease and a clean retry passes", func(t *testing.T) {
		r := newRig(t, outputJob())
		n := &node{t: t, files: outputs(), job: outputJob(), flipChunk: 2}
		if err := r.runSSH(n); err == nil {
			t.Fatal("a corrupt blob did not end the lease")
		}
		r.nothingVisible(outputs())
		r2 := newRig(t, outputJob())
		if err := r2.runSSH(&node{t: t, files: outputs(), job: outputJob()}); err != nil {
			t.Fatal(err)
		}
		r2.assertDelivered(outputs())
	})
	t.Run("a cut send resumes from the reported offset", func(t *testing.T) {
		r := newRig(t, outputJob())
		n := &node{t: t, files: outputs(), job: outputJob(), cutChunk: 2}
		if err := r.runSSH(n); err != nil {
			t.Fatal(err)
		}
		if n.cut != 1 {
			t.Fatalf("the cut was injected %d times", n.cut)
		}
		r.assertDelivered(outputs())
		var announces, offsets []int64
		for _, m := range n.sent() {
			switch b := m.Body.(type) {
			case protocol.Digest:
				announces = append(announces, b.Size)
			case protocol.Chunk:
				offsets = append(offsets, b.Offset)
			}
		}
		// big.bin is announced twice (the second announce after the cut) and its first chunk is not resent;
		// the small file is announced once although two files carry it.
		if !slices.Equal(announces, []int64{1_300_000, 1_300_000, 20}) || !slices.Equal(offsets, []int64{0, 524_288, 1_048_576, 0}) {
			t.Fatalf("announces %v, chunk offsets %v: the resume must continue at the reported offset", announces, offsets)
		}
	})
	t.Run("session loss loses the attempt and shows no partial", func(t *testing.T) {
		r := newRig(t, outputJob())
		n := &node{t: t, files: outputs(), job: outputJob(), dieAtChunk: 2}
		if err := r.runSSH(n); err == nil {
			t.Fatal("a dead ssh session was not reported")
		}
		r.nothingVisible(outputs())
		if state, _, _ := r.st.AttemptStateEpoch(context.Background(), "a1"); state == "passed" || state == "finalizing" {
			t.Fatalf("attempt state %s after the session died", state)
		}
	})
	t.Run("a delivery that stops after the first ref cannot let the job pass", func(t *testing.T) {
		r := newRig(t, outputJob())
		n := &node{t: t, files: outputs(), job: outputJob(), failRefAt: 2}
		if err := r.runSSH(n); err != nil {
			t.Fatal(err)
		}
		if ev := r.snk.(*simSink).lastEvidence(); !strings.Contains(ev, "agent_hook_failed") || !strings.Contains(ev, `"result":"error"`) {
			t.Fatalf("the result of a partial delivery says %s, want an error result", ev)
		}
		staged, _ := r.st.ArtifactStaged(context.Background(), "a1")
		if len(staged) != 1 {
			t.Fatalf("%d records staged, want only the first ref's", len(staged))
		}
	})
	t.Run("an artifact-ref for an undeclared path is refused", func(t *testing.T) {
		r := newRig(t, outputJob())
		small := outputs()["out/small.txt"]
		n := &node{t: t, files: outputs(), job: outputJob(), extraRef: &protocol.ArtifactRef{Path: "secrets/not-declared.txt", SHA256: sum(small), Size: int64(len(small))}}
		err := r.runSSH(n)
		if err == nil || !strings.Contains(err.Error(), "not_declared") {
			t.Fatalf("err = %v, want E712 not_declared", err)
		}
		staged, _ := r.st.ArtifactStaged(context.Background(), "a1")
		for _, s := range staged {
			if strings.HasPrefix(s.Name, "secrets/") {
				t.Fatalf("an undeclared path was staged: %+v", s)
			}
		}
	})
}

func TestRemoteArtifactsAgent(t *testing.T) {
	t.Run("delivered", func(t *testing.T) {
		r := newRig(t, outputJob())
		n := &node{t: t, files: outputs(), job: outputJob()}
		if err := r.runAgent(n); err != nil {
			t.Fatal(err)
		}
		r.assertDelivered(outputs())
		checkFrames(t, n.sent())
	})
	t.Run("a cut at the second chunk resumes and nothing is visible before the final chunk", func(t *testing.T) {
		r := newRig(t, outputJob())
		big := outputs()["out/big.bin"]
		n := &node{t: t, files: outputs(), job: outputJob(), cutChunk: 2}
		visibleBeforeFinal := false
		n.onChunk = func(k int) {
			if k <= 3 && r.has(big) { // chunks 1..3 are big.bin's; the last of them is the final one
				visibleBeforeFinal = true
			}
		}
		if err := r.runAgent(n); err != nil {
			t.Fatal(err)
		}
		if visibleBeforeFinal || n.cut != 1 {
			t.Fatalf("visible before final: %v, cuts injected: %d", visibleBeforeFinal, n.cut)
		}
		r.assertDelivered(outputs())
	})
	t.Run("flipped byte in transit ends the lease and shows nothing", func(t *testing.T) {
		r := newRig(t, outputJob())
		if err := r.runAgent(&node{t: t, files: outputs(), job: outputJob(), flipChunk: 2}); err == nil {
			t.Fatal("a corrupt blob did not end the lease")
		}
		r.nothingVisible(outputs())
	})
	t.Run("an artifact-ref for an undeclared path is refused", func(t *testing.T) {
		r := newRig(t, outputJob())
		r.deadline = 8 * time.Second // the refused frame stays at the head of the pull carrier's queue; the lease ends by the deadline
		small := outputs()["out/small.txt"]
		n := &node{t: t, files: outputs(), job: outputJob(), extraRef: &protocol.ArtifactRef{Path: "secrets/not-declared.txt", SHA256: sum(small), Size: int64(len(small))}}
		if err := r.runAgent(n); err == nil {
			t.Fatal("the lease ended cleanly with an undeclared ref")
		}
		if !r.snk.saw("not_declared") {
			t.Fatal("the handler never refused the undeclared path with E712 not_declared")
		}
		staged, _ := r.st.ArtifactStaged(context.Background(), "a1")
		for _, s := range staged {
			if strings.HasPrefix(s.Name, "secrets/") {
				t.Fatalf("an undeclared path was staged: %+v", s)
			}
		}
	})
}

// TestRemoteArtifactsZombie: the lease lapses (the engine bumps the epoch and loses the attempt)
// while the node is still uploading. Nothing a zombie sends is committed. On carrier A the handler
// is the first line: it refuses every write with E714 stale_epoch and writes one audit row for the
// blob. On carrier B the listener's token and lease gate (E662) refuses the zombie's calls before
// they reach the handler, so the handler's row is not required there.
func TestRemoteArtifactsZombie(t *testing.T) {
	for name, run := range map[string]func(*rig, *node) error{"ssh": (*rig).runSSH, "agent": (*rig).runAgent} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, outputJob())
			n := &node{t: t, files: outputs(), job: outputJob()}
			n.onChunk = func(k int) {
				if k == 2 {
					r.expire() // the lease lapses after the first chunk landed
				}
			}
			if err := run(r, n); err == nil {
				t.Fatal("a zombie's upload was accepted")
			}
			r.nothingVisible(outputs())
			if state, _, _ := r.st.AttemptStateEpoch(context.Background(), "a1"); state == "passed" || state == "finalizing" {
				t.Fatalf("attempt state %s after a zombie upload", state)
			}
			if name == "ssh" {
				if got := r.audits("artifact.remote.refused"); got != 1 {
					t.Fatalf("audit rows = %d, want one for the refused blob", got)
				}
				if r.snk.refused("stale_epoch") == 0 {
					t.Fatal("no write was refused with E714 stale_epoch")
				}
			}
		})
	}
}
