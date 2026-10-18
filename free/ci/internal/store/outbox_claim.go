package store

// Purpose: the delivery claim of the status outbox (P7-CI-37). Statuses of one
// outbox key reach the host in seq order across processes. A claim carries a
// token (its claimed_at stamp); release and posted act only for the claim's
// own token, so a poster that was suspended past the claim TTL and lost its
// claim cannot disturb the one that took over.
// Inputs: outbox key, seq, claim token. Outputs: claim result and token.
// Constraints: one transaction on the writer connection per call. The host
// (a commit status API) is last-write-wins: a stale poster that still
// delivers after losing its claim is healed by re-queueing the newest posted
// status, see OutboxPosted.

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Claim results.
const (
	ClaimOK         = iota // the caller may post
	ClaimSuperseded        // a newer seq is posted or being posted; do not post
	ClaimWait              // an older seq is being posted right now; wait, then retry
	ClaimBusy              // another process is posting this very seq
)

// claimTTL: a posting claim older than this (crashed poster) is taken over.
const claimTTL = 60 * time.Second

// OutboxClaim takes the right to deliver seq so statuses of one key reach the
// host in seq order across processes: refused when a newer seq is posted or
// posting (seq is marked superseded), told to wait while an older seq is
// mid-post. On ClaimOK the returned token fences release and posted.
func (s *Store) OutboxClaim(ctx context.Context, revision, statusContext string, seq int64) (int, int64, error) {
	result, token := ClaimOK, int64(0)
	err := s.write(ctx, func(tx *sql.Tx) error {
		now := s.now()
		cutoff := now.Add(-claimTTL).UnixNano()
		var newer, older int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM outbox WHERE revision=? AND context=? AND seq>? AND (state=? OR (state=? AND claimed_at>?))", revision, statusContext, seq, OutboxPosted, OutboxPosting, cutoff).Scan(&newer); err != nil {
			return err
		}
		if newer > 0 {
			result = ClaimSuperseded
			_, err := tx.ExecContext(ctx, "UPDATE outbox SET state=? WHERE revision=? AND context=? AND seq=? AND state IN (?,?,?)", OutboxSuperseded, revision, statusContext, seq, OutboxReserved, OutboxPending, OutboxPosting)
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM outbox WHERE revision=? AND context=? AND seq<? AND state=? AND claimed_at>?", revision, statusContext, seq, OutboxPosting, cutoff).Scan(&older); err != nil {
			return err
		}
		if older > 0 {
			result = ClaimWait
			return nil
		}
		token = now.UnixNano()
		res, err := tx.ExecContext(ctx, "UPDATE outbox SET state=?,claimed_at=? WHERE revision=? AND context=? AND seq=? AND (state=? OR (state=? AND claimed_at<=?))", OutboxPosting, token, revision, statusContext, seq, OutboxPending, OutboxPosting, cutoff)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			result, token = ClaimBusy, 0
		}
		return nil
	})
	return result, token, err
}

// OutboxRelease gives a claim back after a failed delivery. A claim that was
// taken over meanwhile is left alone.
func (s *Store) OutboxRelease(ctx context.Context, revision, statusContext string, seq, token int64) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE outbox SET state=?,claimed_at=0 WHERE revision=? AND context=? AND seq=? AND state=? AND claimed_at=?", OutboxPending, revision, statusContext, seq, OutboxPosting, token)
		return err
	})
}

// OutboxPosted records a delivery made under the claim token. When the claim
// was taken over, or a newer status was delivered, while the post was in
// flight (the poster was suspended), the delivery landed after a newer
// status: the newest posted status is
// queued again so the next delivery restores the host to it, and E607 tells
// the caller the claim was lost.
func (s *Store) OutboxPosted(ctx context.Context, revision, statusContext string, seq, token int64) error {
	lost := false
	err := s.write(ctx, func(tx *sql.Tx) error {
		var newer int // a newer status delivered while this post was in flight: this one landed last
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM outbox WHERE revision=? AND context=? AND seq>? AND state=?", revision, statusContext, seq, OutboxPosted).Scan(&newer); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, "UPDATE outbox SET state=? WHERE revision=? AND context=? AND seq=? AND state=? AND claimed_at=?", OutboxPosted, revision, statusContext, seq, OutboxPosting, token)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 1 && newer == 0 {
			return nil
		}
		lost = true // committed with the re-queue below: an error would roll it back
		_, err = tx.ExecContext(ctx, "UPDATE outbox SET state=?,claimed_at=0 WHERE revision=? AND context=? AND state=? AND seq=(SELECT max(seq) FROM outbox WHERE revision=? AND context=? AND state=? AND seq>?)", OutboxPending, revision, statusContext, OutboxPosted, revision, statusContext, OutboxPosted, seq)
		return err
	})
	if err == nil && lost {
		return coded("E607", claimLostMsg)
	}
	return err
}

const claimLostMsg = "outbox claim lost while delivering"

// IsClaimLost reports whether err is the E607 answer of OutboxPosted.
func IsClaimLost(err error) bool {
	var se *Error
	return errors.As(err, &se) && se.Code == "E607" && se.Message == claimLostMsg
}
