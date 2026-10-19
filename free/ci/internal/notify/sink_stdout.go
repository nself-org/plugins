package notify

// Purpose: the stdout sink: one JSON line per event.
// Inputs: Deps.Out (default os.Stdout). Outputs: the line, newline-terminated.
// Constraints: one write per event under a lock, so lines from concurrent sinks do not mix.

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
)

type stdoutSink struct {
	name string
	mu   sync.Mutex
	out  io.Writer
}

func newStdout(spec SinkSpec, d Deps) (Notifier, error) {
	if spec.URL != "" || spec.Secret != "" || len(spec.Argv) != 0 {
		return nil, coded("sink " + nameOf(spec) + ": a stdout sink takes no url, secret or argv")
	}
	out := d.Out
	if out == nil {
		out = os.Stdout
	}
	return &stdoutSink{name: nameOf(spec), out: out}, nil
}

func (s *stdoutSink) Name() string { return s.name }

func (s *stdoutSink) Notify(ctx context.Context, ev Event) error {
	if err := ctx.Err(); err != nil {
		return coded("sink " + s.name + ": cancelled")
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return coded("sink " + s.name + ": event not encodable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err = s.out.Write(append(line, '\n')); err != nil {
		return coded("sink " + s.name + ": write failed")
	}
	return nil
}
