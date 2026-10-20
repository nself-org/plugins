package store

// Purpose: the delivery claim of the status outbox (P7-CI-37; P7-HOST-02 counts
// a parked (failed) newer seq as newer too, so no engine posts an older status
// past it). Statuses of one outbox key reach the host in seq order across
// processes. A claim carries a
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
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM outbox WHERE revision=? AND context=? AND seq>? AND (state IN (?,?) OR (state=? AND claimed_at>?))", revision, statusContext, seq, OutboxPosted, OutboxFailed, OutboxPosting, cutoff).Scan(&newer); err != nil {
			return err
		}
		if newer > 0 {
			result = ClaimSuperseded
			// An expired in-flight delivery may have landed after the newer
			// status. Repost that status even if this poster never acknowledges.
			var expired int
			if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM outbox WHERE revision=? AND context=? AND seq=? AND state=? AND claimed_at<=?", revision, statusContext, seq, OutboxPosting, cutoff).Scan(&expired); err != nil {
				return err
			}
			if expired > 0 {
				if _, err := requeueNewest(ctx, tx, revision, statusContext, seq); err != nil {
					return err
				}
			}
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
// taken over meanwhile is left alone. A failed post may still have landed (a
// lost response), so when a newer status is posted or posting the newest one
// is queued again, exactly as OutboxPosted does, and E607 says so.
func (s *Store) OutboxRelease(ctx context.Context, revision, statusContext string, seq, token int64) error {
	lost := false
	err := s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE outbox SET state=?,claimed_at=0 WHERE revision=? AND context=? AND seq=? AND state=? AND claimed_at=?", OutboxPending, revision, statusContext, seq, OutboxPosting, token); err != nil {
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

// OutboxPosted records a delivery made under the claim token. When a newer
// status is posted or being posted (it took our expired claim over while this
// post was in flight, because the poster was suspended), this delivery may
// have landed after it: the newest such status is queued again (its own
// acknowledgement then fails the same way and delivers it once more), so the
// host ends on it, and E607 tells the caller the claim was lost.
func (s *Store) OutboxPosted(ctx context.Context, revision, statusContext string, seq, token int64) error {
	lost := false
	err := s.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE outbox SET state=? WHERE revision=? AND context=? AND seq=? AND state=? AND claimed_at=?", OutboxPosted, revision, statusContext, seq, OutboxPosting, token)
		if err != nil {
			return err
		}
		owned := false
		if n, _ := res.RowsAffected(); n == 1 {
			owned = true
		}
		lost, err = requeueNewest(ctx, tx, revision, statusContext, seq)
		if err != nil {
			return err
		}
		lost = lost || !owned // not ours any more: someone took the claim over
		return nil
	})
	if err == nil && lost {
		return coded("E607", claimLostMsg)
	}
	return err
}

// requeueNewest queues the newest posted-or-posting status above seq again
// and reports whether there was one. The caller commits it with its own
// change, so the answer to the caller can be an error without losing it.
func requeueNewest(ctx context.Context, tx *sql.Tx, revision, statusContext string, seq int64) (bool, error) {
	res, err := tx.ExecContext(ctx, "UPDATE outbox SET state=?,claimed_at=0 WHERE revision=? AND context=? AND state IN (?,?) AND seq=(SELECT max(seq) FROM outbox WHERE revision=? AND context=? AND state IN (?,?) AND seq>?)", OutboxPending, revision, statusContext, OutboxPosted, OutboxPosting, revision, statusContext, OutboxPosted, OutboxPosting, seq)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

const claimLostMsg = "outbox claim lost while delivering"

// IsClaimLost reports whether err is the E607 answer of OutboxPosted.
func IsClaimLost(err error) bool {
	var se *Error
	return errors.As(err, &se) && se.Code == "E607" && se.Message == claimLostMsg
}
