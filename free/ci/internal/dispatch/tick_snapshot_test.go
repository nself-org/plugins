package dispatch

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func TestTickSharesCapacitySnapshot(t *testing.T) {
	for _, releaseDuringBuild := range []bool{false, true} {
		t.Run(map[bool]string{false: "full", true: "release between worlds"}[releaseDuringBuild], func(t *testing.T) {
			st := openStore(t)
			runner := testRunner("local", "local", 1)
			runner.Capability.Lifecycle.AuthorizedProjects = []string{"alpha", "beta"}
			live := liveWorld(st, nil, runner)
			var occupied string
			builds := 0
			d := newDispatcher(t, Config{Store: st, Orderer: FIFO{}, World: func(ctx context.Context, p sched.Pipeline) (sched.World, string, error) {
				w, digest, err := live(ctx, p)
				builds++
				if occupied != "" && releaseDuringBuild && builds == 1 {
					pass(t, st, occupied)
				}
				return w, digest, err
			}})
			first := queueAttempt(t, st, "occupied", "alpha", "running")
			j := jobIn("alpha", first)
			j.Sched.Pipeline.ID = "occupied"
			e, err := d.enqueue(j)
			if err != nil {
				t.Fatal(err)
			}
			if err = d.Tick(context.Background()); err != nil {
				t.Fatal(err)
			}
			if !resolved(e) {
				t.Fatal("occupying job was not admitted")
			}
			occupied, builds = first, 0
			var waiting []*entry
			for _, project := range []string{"alpha", "beta"} {
				id := queueAttempt(t, st, "pipe-"+project, project, "wait")
				j := jobIn(project, id)
				j.Sched.Pipeline.ID = "pipe-" + project
				en, err := d.enqueue(j)
				if err != nil {
					t.Fatal(err)
				}
				waiting = append(waiting, en)
			}
			if err := d.Tick(context.Background()); err != nil {
				t.Fatal(err)
			}
			if resolved(waiting[0]) || resolved(waiting[1]) {
				t.Fatal("a pipeline used a capacity release from between world builds")
			}
			if releaseDuringBuild {
				occupied = ""
				if err := d.Tick(context.Background()); err != nil {
					t.Fatal(err)
				}
				if !resolved(waiting[0]) || resolved(waiting[1]) {
					t.Fatal("next tick did not honor the fair order")
				}
			}
		})
	}
}
