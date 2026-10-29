package fleet

// Purpose: ask every node for its status documents in parallel: at most Parallel at once, each under
// its own hard cap. A reader that ignores cancellation is abandoned at the cap, so one paused host
// costs the pass at most NodeCap and never delays the others.
// Inputs:  the nodes, a Reader and Options.
// Outputs: one Reply per node, in the order of the nodes.
// Constraints: no clock read of its own (the cap comes from the context or the injected Timer).

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/nodes/probe"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
)

// Reader reads the status documents of one node.
type Reader func(ctx context.Context, n registry.Node) (probe.StatusDocs, error)

// Options tune one pass. The zero value uses NodeCap and Parallel.
type Options struct {
	Cap      time.Duration
	Parallel int
	// Timer, when set, is the cap of one node instead of the context deadline (tests fire it).
	Timer func(node string, d time.Duration) <-chan time.Time
	// Now is the clock that ages demand documents; nil means time.Now.
	Now func() time.Time
}

func (o Options) limits() (time.Duration, int) {
	limit, n := o.Cap, o.Parallel
	if limit <= 0 {
		limit = NodeCap
	}
	if n <= 0 {
		n = Parallel
	}
	return limit, n
}

// Reply is the answer of one node.
type Reply struct {
	Docs probe.StatusDocs
	Err  error
}

// errCap marks a node that did not answer within its cap.
var errCap = errors.New("no answer within the cap")

// Ask reads every node through read and returns the replies in node order.
func Ask(ctx context.Context, nodes []registry.Node, read Reader, o Options) []Reply {
	limit, n := o.limits()
	out := make([]Reply, len(nodes))
	slots := make(chan struct{}, n)
	var wg sync.WaitGroup
	for i := range nodes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				out[i] = Reply{Err: ctx.Err()}
				return
			}
			out[i] = askOne(ctx, nodes[i], read, limit, o.Timer)
		}(i)
	}
	wg.Wait()
	return out
}

// askOne runs one read and stops waiting at the cap whether or not the reader returns.
func askOne(ctx context.Context, node registry.Node, read Reader, limit time.Duration, timer func(string, time.Duration) <-chan time.Time) Reply {
	if read == nil {
		return Reply{Err: fmt.Errorf("no status reader")}
	}
	cctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	var deadline <-chan time.Time // nil: the context deadline alone is the cap
	if timer != nil {
		deadline = timer(node.Record.ID, limit)
	}
	answer := make(chan Reply, 1) // buffered: an abandoned reader never blocks on send
	go func() {
		docs, err := read(cctx, node)
		answer <- Reply{Docs: docs, Err: err}
	}()
	select {
	case r := <-answer:
		if r.Err != nil && errors.Is(cctx.Err(), context.DeadlineExceeded) {
			r.Err = fmt.Errorf("%w (%s)", errCap, limit)
		}
		return r
	case <-deadline:
		return Reply{Err: fmt.Errorf("%w (%s)", errCap, limit)}
	case <-cctx.Done():
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return Reply{Err: fmt.Errorf("%w (%s)", errCap, limit)}
		}
		return Reply{Err: cctx.Err()}
	}
}

// reason is the one-line, bounded text of a failed read.
func reason(err error) string {
	s := strings.TrimSpace(err.Error())
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
