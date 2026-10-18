package hygiene

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// TestInvariant_I17: outside the Epic D12 exception, the UID chosen for a job is never the agent's
// or the coordinator's, whatever the world and however the node is provisioned.
func TestInvariant_I17(t *testing.T) {
	invariants.Check(t, func(w invariants.World) error {
		for _, prov := range []struct{ shim, userns, operatorNode bool }{
			{false, false, false}, {true, false, false}, {false, true, false}, {true, true, false},
			{false, false, true}, {true, false, true}, {false, true, true}, {true, true, true},
		} {
			f := Facts{Isolation: w.Isolation, Trust: w.Trust, Coordinator: trust.DecodeCoordinatorMode(w.Mode), Ownership: w.Ownership,
				Slots: 3, AgentUID: agentUID, CoordinatorUID: coordUID, OperatorNode: prov.operatorNode}
			if prov.shim {
				f.JobShim, f.JobUIDs = true, []int{2000, 2001, 2002}
			}
			if prov.userns {
				f.Userns, f.SubUIDStart, f.SubUIDCount = true, 100000, 65536
			}
			for slot := 0; slot < 3; slot++ {
				p, err := Separation(f, slot)
				if err != nil {
					if codeOf(err) != CodeNoJobUID {
						return fmt.Errorf("uncoded refusal %v", err)
					}
					continue
				}
				if p.UID != agentUID && p.UID != coordUID {
					continue
				}
				exception := w.Trust == "owner" && w.Isolation == "process" && w.Mode == "personal" && w.Ownership == "operator" && prov.operatorNode
				if p.Mode != OwnerException || !exception {
					return fmt.Errorf("uid %d (%s) handed out outside the exception: %+v", p.UID, p.Mode, f)
				}
			}
		}
		return nil
	})
}

// TestInvariant_I08: attempt N+1 observes no file, container, network or volume of attempt N.
// What a job can leave behind varies with its trust class, isolation level and network scope (files
// and symlinks, container, network and volume resources), so the real filesystem and runtime-fake
// round trip runs once per distinct (trust, isolation, network) shape and every world of the
// enumeration and the generated set is checked against that shape's outcome.
func TestInvariant_I08(t *testing.T) {
	base := t.TempDir()
	rt := newFakeRuntime()
	h := &Host{Base: filepath.Join(base, "work"), Home: base, Runtime: rt, ScanDirs: []string{}}
	type shape struct {
		trust model.TrustClass
		level model.Isolation
		net   model.NetworkScope
	}
	outcomes := map[shape]error{}
	n := 0
	invariants.Check(t, func(w invariants.World) error {
		key := shape{w.Trust, w.Isolation, w.Network}
		if err, seen := outcomes[key]; seen {
			return err
		}
		n++
		err := successorSeesNothing(h, rt, base, fmt.Sprintf("prev%d", n), fmt.Sprintf("next%d", n), key.trust, key.level)
		outcomes[key] = err
		return err
	})
	if n < 20 {
		t.Fatalf("only %d distinct shapes were exercised", n)
	}
}

// successorSeesNothing leaves state for attempt prev, cleans it, and checks that attempt next starts clean.
func successorSeesNothing(h *Host, rt *fakeRuntime, base, prev, next string, tr model.TrustClass, level model.Isolation) error {
	root, err := h.JobRoot(prev)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "tmp", string(tr)), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "tmp", "env"), []byte(string(level)), 0o600); err != nil {
		return err
	}
	if err := os.Symlink(base, filepath.Join(root, "home")); err != nil {
		return err
	}
	rt.add(exec.KindContainer, "c"+prev, prev)
	rt.add(exec.KindNetwork, "n"+prev, prev)
	rt.add(exec.KindVolume, "v"+prev, prev)
	if err := h.Cleanup(context.Background(), prev); err != nil {
		return fmt.Errorf("cleanup: %w", err)
	}
	if rep, err := h.LeakCheck(context.Background(), prev, 0); err != nil || !rep.Clean() {
		return fmt.Errorf("leak after cleanup: %v %+v", err, rep)
	}
	nroot, err := h.JobRoot(next)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(nroot)
	if err != nil || len(entries) != 0 {
		return fmt.Errorf("attempt N+1 root is not empty: %v %v", entries, err)
	}
	if remaining, _ := os.ReadDir(h.Base); len(remaining) != 1 {
		return fmt.Errorf("work dir holds %d entries, want only the new root", len(remaining))
	}
	for _, kind := range Kinds {
		if got, _ := rt.List(context.Background(), kind, ""); len(got) != 0 {
			return fmt.Errorf("%s of N survived: %v", kind, got)
		}
	}
	if _, err := os.Stat(base); err != nil {
		return fmt.Errorf("the symlink target was damaged: %w", err)
	}
	return h.Cleanup(context.Background(), next)
}
