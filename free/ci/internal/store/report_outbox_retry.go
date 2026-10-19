package store

// Purpose: retry, collapse and ownership columns of the status outbox
// (P7-HOST-02, Epic D7). A failed delivery is parked as `failed` with an
// attempt count and the time of the next attempt; an entry the reporting owner
// does not deliver is `suppressed` with its reason; a lower seq that a newer
// one replaces is `superseded`. Migration set report-outbox, file 0003.
// Inputs: outbox key (revision, context), seq, claim token, clock time.
// Outputs: due entries, listings, new seq numbers.
// Constraints: SQL only here; one transaction per call on the writer
// connection; payloads carry no secrets (the Status JSON).

import (
	"context"
	"database/sql"
	"time"
)

const ( // outbox entry states added by P7-HOST-02
	OutboxFailed     = "failed"     // a delivery failed; retried when next_attempt_at passes
	OutboxSuppressed = "suppressed" // the reporting owner is another process; never posted here
)

// OutboxItem is one outbox entry with its retry bookkeeping.
type OutboxItem struct {
	Revision, Context string
	Seq               int64
	State             string
	Payload           []byte
	Attempts          int
	NextAttemptAt     int64 // unix nanoseconds; 0 when none is scheduled
	LastError, Reason string
	CreatedAt         int64
}

const outboxItemCols = "revision,context,seq,state,coalesce(payload_json,x''),attempts,next_attempt_at,last_error,reason,created_at"

func scanOutboxItem(r *sql.Rows) (e OutboxItem, err error) {
	err = r.Scan(&e.Revision, &e.Context, &e.Seq, &e.State, &e.Payload, &e.Attempts, &e.NextAttemptAt, &e.LastError, &e.Reason, &e.CreatedAt)
	return
}

// OutboxDue collapses the outbox, makes every failed entry whose time has come
// deliverable again and lists what can be delivered now (pending, or posting
// past its claim TTL), oldest first. Collapse: only the highest undelivered
// seq of a (revision, context) stays; the lower pending or failed ones are
// superseded, whether or not the higher one is due (CI D13, HOST D7).
func (s *Store) OutboxDue(ctx context.Context, now time.Time) ([]OutboxItem, error) {
	err := s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE outbox SET state=? WHERE state IN (?,?) AND EXISTS (SELECT 1 FROM outbox n WHERE n.revision=outbox.revision AND n.context=outbox.context AND n.seq>outbox.seq AND n.state IN (?,?))", OutboxSuperseded, OutboxPending, OutboxFailed, OutboxPending, OutboxFailed); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "UPDATE outbox SET state=? WHERE state=? AND next_attempt_at<=?", OutboxPending, OutboxFailed, now.UnixNano())
		return err
	})
	if err != nil {
		return nil, err
	}
	return collect(ctx, s, scanOutboxItem, "SELECT "+outboxItemCols+" FROM outbox WHERE state=? OR (state=? AND claimed_at<=?) ORDER BY created_at,seq", OutboxPending, OutboxPosting, now.Add(-claimTTL).UnixNano())
}

// OutboxList lists the entries an operator needs to see: pending, failed and
// suppressed, oldest first.
func (s *Store) OutboxList(ctx context.Context) ([]OutboxItem, error) {
	return collect(ctx, s, scanOutboxItem, "SELECT "+outboxItemCols+" FROM outbox WHERE state IN (?,?,?) ORDER BY created_at,seq", OutboxPending, OutboxFailed, OutboxSuppressed)
}

// OutboxEnqueue takes the next seq for (revision, context) and stores the
// payload as a deliverable entry in one step (Repost, P7-HOST-02).
func (s *Store) OutboxEnqueue(ctx context.Context, revision, statusContext string, payload []byte) (int64, error) {
	var seq int64
	err := s.write(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT coalesce(max(seq),0)+1 FROM outbox WHERE revision=? AND context=?", revision, statusContext).Scan(&seq); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO outbox(revision,context,seq,state,payload_json,created_at) VALUES (?,?,?,?,?,?)", revision, statusContext, seq, OutboxPending, payload, s.now().UnixNano())
		return err
	})
	return seq, err
}

// OutboxBackoff parks a claimed entry after a failed delivery until next. Like
// OutboxRelease it queues the newest posted status again when the failed post
// may have landed after it, and then answers E607.
func (s *Store) OutboxBackoff(ctx context.Context, revision, statusContext string, seq, token int64, attempts int, next time.Time, cause string) error {
	lost := false
	err := s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE outbox SET state=?,claimed_at=0,attempts=?,next_attempt_at=?,last_error=? WHERE revision=? AND context=? AND seq=? AND state=? AND claimed_at=?", OutboxFailed, attempts, next.UnixNano(), cause, revision, statusContext, seq, OutboxPosting, token); err != nil {
			return err
		}
		var err error
		lost, err = requeueNewest(ctx, tx, revision, statusContext, seq)
		return err
	})
	if err == nil && lost {
		return coded("E607", claimLostMsg)
	}
	return err
}

// OutboxSuppress parks an undelivered entry that this process must not post
// (another reporter owns it, or its payload cannot be read). A posting entry
// is only listed by OutboxDue when its claim has expired.
func (s *Store) OutboxSuppress(ctx context.Context, revision, statusContext string, seq int64, reason string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE outbox SET state=?,reason=?,claimed_at=0 WHERE revision=? AND context=? AND seq=? AND state IN (?,?,?)", OutboxSuppressed, reason, revision, statusContext, seq, OutboxPending, OutboxFailed, OutboxPosting)
		return err
	})
}

// OutboxRequeue makes a suppressed entry deliverable again.
func (s *Store) OutboxRequeue(ctx context.Context, revision, statusContext string, seq int64) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE outbox SET state=?,reason='' WHERE revision=? AND context=? AND seq=? AND state=?", OutboxPending, revision, statusContext, seq, OutboxSuppressed)
		return err
	})
}
