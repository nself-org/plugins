package audit

// Purpose: read-only verification of the chain (Epic D20): `serve admin audit --verify`.
// Inputs:  the store and the sealer interval (default 60 s).
// Outputs: a Report; OK is false when a sealed row does not match its hash, a link is broken, or unsealed
//          rows are older than the interval.
// Constraints: never writes and never seals; an edited sealed row is named by seq; deleting a middle row
//          breaks the link at the next row; an unsealed row inside the interval is reported but not a failure.

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// SealInterval is how long a row may stay unsealed before verification fails.
const SealInterval = 60 * time.Second

// Report is the result of Verify.
type Report struct {
	Rows, Sealed, Unsealed, Overdue int
	BadSeq                          int64
	Reason                          string
}

// OK reports a verified chain with nothing overdue.
func (r Report) OK() bool { return r.BadSeq == 0 && r.Overdue == 0 }

// Verify walks the whole table in seq order.
func (l *Log) Verify(ctx context.Context, interval time.Duration) (Report, error) {
	if interval <= 0 {
		interval = SealInterval
	}
	var rep Report
	var prev []byte
	var after int64
	cutoff := l.St.Now() - interval.Nanoseconds()
	for {
		page, err := l.St.AuditEntries(ctx, after, 1000)
		if err != nil {
			return rep, err
		}
		if len(page) == 0 {
			return rep, nil
		}
		for _, e := range page {
			after = e.Seq
			rep.Rows++
			if e.Head == nil {
				rep.Unsealed++
				if e.TS <= cutoff {
					rep.Overdue++
				}
				continue
			}
			rep.Sealed++
			if rep.BadSeq != 0 {
				prev = e.Head
				continue
			}
			if reason := checkEntry(prev, e); reason != "" {
				rep.BadSeq, rep.Reason = e.Seq, reason
			}
			prev = e.Head
		}
	}
}

func checkEntry(prev []byte, e store.AuditEntry) string {
	if !bytes.Equal(e.PrevHead, prev) {
		return "chain link broken: prev_head does not match the previous sealed row"
	}
	want, err := Hash(prev, e.AuditRow)
	if err != nil {
		return fmt.Sprintf("row cannot be canonicalised: %v", err)
	}
	if !bytes.Equal(want, e.Head) {
		return "row was changed after sealing: head does not match its content"
	}
	return ""
}
