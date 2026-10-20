package coordclient

// Purpose: the event stream of a remote run with reconnect by seq (P7-NODE-19, Epic D14): GET
// /v1/runs/{id}/events carries contract:ci.events NDJSON; when the connection breaks the client asks again
// with from=lastSeq+1 and the coordinator, which rebuilds the same records for the same run, resumes there.
// Inputs:  a run reference and an emit function for the raw lines.
// Outputs: every record exactly once, in seq order, ending with the result (or error) record.
// Constraints: a record below the next expected seq is dropped (never emitted twice), one above it is a
//          gap and forces a reconnect from the expected seq (never emitted early); the final record is
//          always accepted; E664 refusals do not retry; consecutive attempts without progress are
//          bounded (about 90 s); an error from emit ends the stream at once; a result that jumps ahead of missing records is a gap too; the wait between attempts grows from 100 ms to 5 s and stops with the context.

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/detach"
)

// Reconnect tunes Events; tests shorten it.
var Reconnect = struct {
	MaxIdle        int           // consecutive attempts that delivered nothing
	First, Longest time.Duration // first and longest wait between attempts
}{MaxIdle: 24, First: 100 * time.Millisecond, Longest: 5 * time.Second}

// Events streams the records of the run. emit gets each raw line (no newline; valid until emit returns)
// and its record; the final record seen is returned.
func (c *Client) Events(ctx context.Context, ref string, emit func(line []byte, rec detach.Record) error) (detach.Record, error) {
	next, idle, wait := int64(1), 0, Reconnect.First
	for {
		rec, progressed, final, err := c.eventsOnce(ctx, ref, &next, emit)
		if final {
			return rec, nil
		}
		if err != nil {
			var ee *emitError
			if errors.As(err, &ee) { // the caller's output broke (a closed pipe): retrying cannot help
				return detach.Record{}, ee.err
			}
			if clientapi.ReasonOf(err) != "" && clientapi.ReasonOf(err) != clientapi.ReasonTransport || ctx.Err() != nil {
				return detach.Record{}, err
			}
		}
		if progressed {
			idle, wait = 0, Reconnect.First
		} else if idle++; idle >= Reconnect.MaxIdle {
			return detach.Record{}, clientapi.Failf(clientapi.ReasonTransport, "the event stream of %s keeps breaking", ref)
		}
		select {
		case <-ctx.Done():
			return detach.Record{}, ctx.Err()
		case <-time.After(wait):
		}
		wait = min(wait*2, Reconnect.Longest)
	}
}

// emitError wraps a failure of the caller's emit function.
type emitError struct{ err error }

func (e *emitError) Error() string { return e.err.Error() }

// eventsOnce reads one connection. next is advanced past every emitted record.
func (c *Client) eventsOnce(ctx context.Context, ref string, next *int64, emit func([]byte, detach.Record) error) (last detach.Record, progressed, final bool, err error) {
	path := clientapi.PathRunsPfx + url.PathEscape(refOrLatest(ref)) + "/events?from=" + strconv.FormatInt(*next, 10)
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return last, false, false, err
	}
	defer func() { _ = resp.Body.Close() }()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		rec, ok := detach.ParseLine(sc.Bytes())
		if !ok {
			continue // not a ci.events record: never forwarded
		}
		switch {
		case rec.Final() && !rec.Err && rec.Seq > *next:
			return last, progressed, false, nil // the result arrived ahead of missing records: ask again
		case rec.Final():
			if err = emit(rec.Raw, rec); err != nil {
				return rec, progressed, false, &emitError{err}
			}
			return rec, true, true, nil
		case rec.Seq < *next:
			continue // already delivered before the break
		case rec.Seq > *next:
			return last, progressed, false, nil // a gap: ask again from the expected seq
		}
		if err = emit(rec.Raw, rec); err != nil {
			return rec, progressed, false, &emitError{err}
		}
		last, progressed = rec, true
		*next = rec.Seq + 1
	}
	return last, progressed, false, nil
}
