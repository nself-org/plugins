package run

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/notify"
)

func TestNotifyRecoveryForDetachedServedRef(t *testing.T) {
	for _, ref := range []string{"main", "refs/heads/release"} {
		t.Run(ref, func(t *testing.T) {
			log := &sinkLog{}
			registerTestSink(t, func(_ context.Context, e notify.Event) error { log.add(e); return nil })
			h := hookFor(openState(t), oneSink)
			h.Ref = nil // Capture returns a detached checkout, with no symbolic ref.
			root := ""
			for _, pass := range []bool{false, true} {
				r := failRig(t)
				if pass {
					r = passRig(t)
				}
				if root == "" {
					root = r.C.Opts.Root
				} else {
					r.C.Opts.Root = root
				}
				r.C.Opts.Ref = ref
				r.C.Hooks.Auto.Notify = h
				r.run(t)
			}
			if len(log.events) != 2 || log.events[0].Kind != notify.KindFailure || log.events[1].Kind != notify.KindRecovery {
				t.Fatalf("events: %v", log.kinds())
			}
			for _, e := range log.events {
				if e.Ref != ref {
					t.Fatalf("ref=%q want=%q", e.Ref, ref)
				}
			}
		})
	}
}
