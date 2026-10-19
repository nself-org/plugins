package run

// Purpose: D-1059 regression. `queued` is the first event of a run and
// `nself ci --detach` gives it 30 s; status preparation (gh resolve, outbox
// drain, reserve) can wait minutes, so it must not sit in front of `queued`.

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// eventTap is a renderer that reports each event type on a channel.
type eventTap struct{ types chan string }

func (e eventTap) Event(ev Event) { e.types <- ev.Type }
func (e eventTap) Done(Summary)   {}

func TestQueuedIsAnnouncedWhileStatusPreparationIsStuck(t *testing.T) {
	// not parallel: RegisterRenderer is process-wide
	tap := eventTap{types: make(chan string, 64)}
	RegisterRenderer("d1059", func(io.Writer, Options) Renderer { return tap })
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Opts.Renderer = "d1059"

	release := make(chan struct{})
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	t.Cleanup(free)
	base := gitFake{}.capture
	r.C.Capture = func(ctx context.Context, argv ...string) (string, int, error) {
		if strings.Contains(strings.Join(argv, " "), "remote get-url") {
			<-release // a slow gh / outbox phase
		}
		return base(ctx, argv...)
	}

	done := make(chan error, 1)
	go func() { _, err := r.C.Run(context.Background()); done <- err }()

	deadline := time.After(5 * time.Second)
	for queued := false; !queued; {
		select {
		case ty := <-tap.types:
			queued = ty == "queued"
		case err := <-done:
			t.Fatalf("run ended (err %v) before queued was announced", err)
		case <-deadline:
			t.Fatal("queued was not announced while status preparation was stuck")
		}
	}
	free()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("run did not finish after status preparation was released")
	}
	if posted := r.Rep.all(); len(posted) != 1 {
		t.Fatalf("the status must still be posted once, got %+v", posted)
	}
}
