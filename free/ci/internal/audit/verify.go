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

// Verify walks the whole table in seq order inside one read snapshot. Besides each row's hash and link it
// checks that the seq numbers are continuous from 1 (rows are never deleted, so a gap, or an AUTOINCREMENT
// counter above the last row, means rows were removed) and that no unsealed row sits below a sealed one
// (sealing runs in seq order, so that means a head was cleared).
func (l *Log) Verify(ctx context.Context, interval time.Duration) (Report, error) {
	if interval <= 0 {
		interval = SealInterval
	}
	snap, err := l.St.AuditSnapshot(ctx)
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = snap.Close() }()
	var rep Report
	var prev []byte
	var last, firstUnsealed int64
	bad := func(seq int64, reason string) {
		if rep.BadSeq == 0 {
			rep.BadSeq, rep.Reason = seq, reason
		}
	}
	cutoff := l.St.Now() - interval.Nanoseconds()
	for {
		page, err := snap.Entries(ctx, last, 1000)
		if err != nil {
			return rep, err
		}
		if len(page) == 0 {
			break
		}
		for _, e := range page {
			rep.Rows++
			if e.Seq != last+1 {
				bad(last+1, fmt.Sprintf("row %d is missing (the next row is %d): rows were removed", last+1, e.Seq))
			}
			last = e.Seq
			if e.Head == nil {
				rep.Unsealed++
				if e.TS <= cutoff {
					rep.Overdue++
				}
				if firstUnsealed == 0 {
					firstUnsealed = e.Seq
				}
				continue
			}
			rep.Sealed++
			if firstUnsealed != 0 {
				bad(firstUnsealed, "row is unsealed below a sealed row: its head was cleared")
			}
			if reason := checkEntry(prev, e); reason != "" {
				bad(e.Seq, reason)
			}
			prev = e.Head
		}
	}
	high, err := snap.HighSeq(ctx)
	if err != nil {
		return rep, err
	}
	if high > last {
		bad(last+1, fmt.Sprintf("row %d was issued but is missing: the newest rows were removed", last+1))
	}
	return rep, nil
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
