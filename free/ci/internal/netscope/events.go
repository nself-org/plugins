package netscope

// Purpose: the structured event the proxy prints for each refusal and the planner reads back.
// Inputs:  a refused CONNECT or request: scope, host, address, reason.
// Outputs: one JSON line per event on the sidecar's stdout; ParseEvents reads them from the sidecar log.
// Constraints: one denial is one event; request bodies, headers and paths are never recorded (only the
// destination host and port), and a line that does not parse is ignored rather than trusted.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// EventDenied is the kind of a refusal event.
const EventDenied = "network.denied"

// EventReady is the one line the sidecar prints once it listens: the planner waits for it before the job starts.
const EventReady = "network.ready"

// ReadyLine is the sidecar's readiness line.
func ReadyLine() []byte { return Event{Kind: EventReady}.Encode() }

// HasReady reports whether a sidecar log holds the readiness line.
func HasReady(log []byte) bool {
	return bytes.Contains(log, []byte(`"kind":"`+EventReady+`"`))
}

// Event is one refusal.
type Event struct {
	Kind    string `json:"kind"`
	At      string `json:"at"`
	Scope   string `json:"scope"`
	Host    string `json:"host"`
	Port    string `json:"port,omitempty"`
	Addr    string `json:"addr,omitempty"`
	Reason  Reason `json:"reason"`
	Detail  string `json:"detail,omitempty"`
	Attempt string `json:"attempt,omitempty"`
}

// maxEventLine bounds a line the planner will parse, so a flood of output cannot grow memory.
const maxEventLine = 4096

// Encode renders the event as one line.
func (e Event) Encode() []byte {
	if e.Kind == "" {
		e.Kind = EventDenied
	}
	if e.At == "" {
		e.At = time.Now().UTC().Format(time.RFC3339Nano)
	}
	b, _ := json.Marshal(e)
	return append(b, '\n')
}

// ParseEvents extracts the denial events from a sidecar log. Other lines are skipped.
func ParseEvents(r io.Reader) []Event {
	var out []Event
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), maxEventLine)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var e Event
		if json.Unmarshal(line, &e) != nil || e.Kind != EventDenied || e.Reason == "" {
			continue
		}
		e.Host = strings.TrimSpace(e.Host)
		out = append(out, e)
	}
	return out
}
