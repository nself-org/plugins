package detach

// Purpose: replay and tail a detached run's event file (P7-CI-63): the body of
// `nself ci runs follow`. The records are copied byte for byte, so seq and the
// final evidence line are exactly what the run wrote.
// Inputs:  the stream file, the first seq wanted, a "the run is over" probe.
// Outputs: the wanted lines on the writer, one per line.
// Constraints: stops after the final line (the result or an error envelope);
// when the file stays idle for IdleGrace and the probe says the run is over
// and no final line came (the child was killed), it reports EndGone and the
// caller closes the stream; lines that are not v1 envelopes (stderr) are never
// copied; the context ends the follow.

import (
	"context"
	"io"
	"time"
)

// IdleGrace is how long the file must stay quiet before Over is asked: the
// store marks a run finished a moment before the last line is written.
const IdleGrace = 2 * time.Second

// End says why a follow stopped.
type End int

// The ways a follow ends.
const (
	EndResult End = iota // the final result line was copied
	EndError             // an error envelope was copied (the run's stream failed)
	EndGone              // the run is over but its stream has no final line
)

// FollowParams configures Follow.
type FollowParams struct {
	Path string
	From int64                                   // first seq to copy (0 = all)
	Poll time.Duration                           // default 100 ms
	Idle time.Duration                           // default IdleGrace
	Over func(quiet time.Duration) (bool, error) // true when the run can produce no more lines; quiet is how long the file has been silent
}

// Outcome is how a follow ended: Exit is the error envelope's exit status for
// EndError, LastSeq the highest seq copied (0 when none).
type Outcome struct {
	End     End
	Exit    int
	LastSeq int64
}

// Follow copies the stream to w and reports how it ended.
func Follow(ctx context.Context, w io.Writer, p FollowParams) (Outcome, error) {
	poll, idle := p.Poll, p.Idle
	if poll <= 0 {
		poll = 100 * time.Millisecond
	}
	if idle <= 0 {
		idle = IdleGrace
	}
	r := &Reader{Path: p.Path}
	defer func() { _ = r.Close() }()
	last, gone, lastSeq := time.Now(), false, int64(0)
	for {
		lines, err := r.Next()
		if err != nil {
			return Outcome{LastSeq: lastSeq}, err
		}
		if len(lines) > 0 {
			last = time.Now()
		}
		for _, l := range lines {
			rec, ok := ParseLine(l)
			if !ok || (!rec.Final() && rec.Seq < p.From) {
				continue
			}
			if _, err = w.Write(append(append([]byte(nil), l...), '\n')); err != nil {
				return Outcome{LastSeq: lastSeq}, err
			}
			lastSeq = max(lastSeq, rec.Seq)
			switch {
			case rec.Err:
				return Outcome{EndError, rec.Exit, lastSeq}, nil
			case rec.Type == "result":
				return Outcome{EndResult, 0, lastSeq}, nil
			}
		}
		if len(lines) == 0 && gone {
			return Outcome{EndGone, 0, lastSeq}, nil // read once more after the probe said over: nothing came
		}
		if len(lines) == 0 && p.Over != nil && time.Since(last) >= idle {
			over, err := p.Over(time.Since(last))
			if err != nil {
				return Outcome{LastSeq: lastSeq}, err
			}
			if gone = over; over {
				continue
			}
		}
		select {
		case <-ctx.Done():
			return Outcome{LastSeq: lastSeq}, ctx.Err()
		case <-time.After(poll):
		}
	}
}
