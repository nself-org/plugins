package world

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestWorldRaceAdmissions(t *testing.T) {
	d, p := worldFixture(t)
	slots := 2010
	d.Local.Availability.Slots.Value = &slots
	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("race-%04d", i)
		if err := d.Store.CreateJob(ctx, store.JobRow{ID: "job-" + id, PipelineID: p.ID, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
			t.Fatal(err)
		}
		if err := d.Store.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "job-" + id, N: 1}, ""); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	done := make(chan struct{})
	fail := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			w, _, err := Build(ctx, d, p)
			if err != nil {
				select {
				case fail <- err:
				default:
				}
				return
			}
			if w.Counts.Global < 5 || w.Counts.Global > 1005 {
				select {
				case fail <- fmt.Errorf("torn count %d", w.Counts.Global):
				default:
				}
				return
			}
			local := w.Runners[len(w.Runners)-1]
			if local.ID != "local" || local.Avail.Slots != 2010-2*w.Counts.Global {
				select {
				case fail <- fmt.Errorf("torn snapshot: count=%d slots=%d", w.Counts.Global, local.Avail.Slots):
				default:
				}
				return
			}
		}
	}()
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("race-%04d", i)
		err := d.Store.AdmitAndLease(ctx, "race-host", store.Demand{AttemptID: id, RunnerID: "local", LeaseID: "lease-" + id, CPU: 1, MemMB: 1, CapacityCPU: 2000, CapacityMemMB: 2000})
		if err != nil {
			close(done)
			wg.Wait()
			t.Fatal(err)
		}
	}
	close(done)
	wg.Wait()
	select {
	case err := <-fail:
		t.Fatal(err)
	default:
	}
	w, _, err := Build(ctx, d, p)
	if err != nil || w.Counts.Global != 1005 {
		t.Fatalf("final count %d: %v", w.Counts.Global, err)
	}
}
