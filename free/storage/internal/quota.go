package internal

import (
	"context"
	"errors"
	"net/http"
)

// Errors planPut returns; the handler maps both to 413.
var (
	errObjectTooLarge = errors.New("object too large")
	errQuotaExceeded  = errors.New("account quota exceeded")
)

// Bodies for the two 413 answers.
const (
	errBodyObjectTooLarge = `{"error":"object too large"}`
	errBodyQuotaExceeded  = `{"error":"account quota exceeded"}`
)

// putPlan is the size budget of one PUT.
type putPlan struct {
	// limit is the most body bytes the upload may carry: min(max object size, quota room).
	limit int64
	// quotaBinds is true when the quota room is what sets limit, so an over-limit body means "quota exceeded".
	quotaBinds bool
}

// planPut decides how many bytes a PUT may carry. Pure: no I/O.
//
//	maxObject  per-object cap (> 0)
//	quota      per-account cap, 0 = off
//	used       bytes the account stores now (ignored when quota is 0)
//	overwrite  size of the row this PUT replaces (0 when the key is new)
//	declared   Content-Length, or -1 when unknown (chunked)
//
// A declared length over maxObject is errObjectTooLarge; over the quota room (quota - (used - overwrite)) is
// errQuotaExceeded. When the length is unknown, or declared but understated, the returned limit still stops the body
// at the budget.
//
// `used` is the stored total read before the upload starts. Every PUT between that lookup and its metadata row
// commit (including PUTs still waiting for an upload slot) sees the same total, so concurrent uploads can overshoot
// the quota by up to the sum of their sizes. This is a best-effort guard against runaway use, not a hard
// reservation. A failed metadata write after an overwrite leaves the old size counted until the row is rewritten.
func planPut(maxObject, quota, used, overwrite, declared int64) (putPlan, error) {
	if declared > maxObject {
		return putPlan{}, errObjectTooLarge
	}
	plan := putPlan{limit: maxObject}
	if quota <= 0 {
		return plan, nil
	}
	room := quota - (used - overwrite)
	if room < 0 {
		room = 0
	}
	if declared > room {
		return putPlan{}, errQuotaExceeded
	}
	if room < maxObject {
		plan.limit, plan.quotaBinds = room, true
	}
	return plan, nil
}

// writePutLimit answers 413 for errObjectTooLarge or errQuotaExceeded.
func writePutLimit(w http.ResponseWriter, err error) {
	body := errBodyObjectTooLarge
	if errors.Is(err, errQuotaExceeded) {
		body = errBodyQuotaExceeded
	}
	http.Error(w, body, http.StatusRequestEntityTooLarge)
}

// isTooLarge reports whether err is the http.MaxBytesReader limit error (it survives the %w wrapping in s3store).
func isTooLarge(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// maxObjectBytes is the effective per-object cap (the default when the config is absent, as in tests).
func (s *Server) maxObjectBytes() int64 {
	if s.cfg == nil || s.cfg.MaxObjectBytes < 1 {
		return defaultMaxObjectBytes
	}
	return s.cfg.MaxObjectBytes
}

// accountQuotaBytes is the per-account quota, 0 when off.
func (s *Server) accountQuotaBytes() int64 {
	if s.cfg == nil {
		return 0
	}
	return s.cfg.AccountQuotaBytes
}

// putLookupResult is what a PUT needs from the database before it writes to S3.
type putLookupResult struct {
	bucketID string
	oldSize  *int64 // size of the row this PUT replaces; nil when the key is new
	used     int64  // bytes the account stores now; 0 unless a quota is on
}

// lookupPut runs the one PUT query: the bucket (pgx.ErrNoRows when it is not this account's), the size of the row
// being overwritten, and, only when quota > 0, the account's stored bytes (served by idx_np_storage_objects_source).
func (s *Server) lookupPut(ctx context.Context, sai, bucket, key string, quota int64) (putLookupResult, error) {
	if s.putLookup != nil {
		return s.putLookup(ctx, sai, bucket, key, quota)
	}
	var res putLookupResult
	err := s.db.QueryRow(ctx,
		`SELECT b.id,
		        (SELECT o.size_bytes FROM np_storage_objects o WHERE o.bucket_id = b.id AND o.key = $3),
		        CASE WHEN $4::bigint > 0
		             THEN (SELECT COALESCE(SUM(a.size_bytes), 0)::bigint FROM np_storage_objects a WHERE a.source_account_id = $1)
		             ELSE 0::bigint END
		 FROM np_storage_buckets b WHERE b.source_account_id = $1 AND b.name = $2`,
		sai, bucket, key, quota).Scan(&res.bucketID, &res.oldSize, &res.used)
	return res, err
}
