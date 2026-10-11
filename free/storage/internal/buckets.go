package internal

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
)

var ipShaped = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$`)

// validBucketName applies the S3 bucket naming rules: 3-63 chars of [a-z0-9.-], starting and ending
// alphanumeric, no "..", not shaped like an IPv4 address. The logical bucket name is the physical S3 bucket.
func validBucketName(name string) bool {
	if len(name) < 3 || len(name) > 63 || strings.Contains(name, "..") || ipShaped.MatchString(name) {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		alnum := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if !alnum && c != '.' && c != '-' {
			return false
		}
		if !alnum && (i == 0 || i == len(name)-1) {
			return false
		}
	}
	return true
}

// handleCreateBucket creates the S3 bucket (idempotent) and records the account's bucket row.
func (s *Server) handleCreateBucket(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	var body struct {
		Name   string `json:"name"`
		Region string `json:"region"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if !validBucketName(body.Name) {
		http.Error(w, `{"error":"invalid bucket name"}`, http.StatusBadRequest)
		return
	}
	if body.Region == "" {
		body.Region = "us-east-1"
	}
	if s.store == nil {
		http.Error(w, errNoStore, http.StatusServiceUnavailable)
		return
	}
	if err := s.store.EnsureBucket(r.Context(), body.Name); err != nil {
		http.Error(w, errStoreFailed, http.StatusBadGateway)
		return
	}

	var id string
	err := s.db.QueryRow(r.Context(),
		`INSERT INTO np_storage_buckets (source_account_id, name, region)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (source_account_id, name) DO UPDATE SET region = EXCLUDED.region
		 RETURNING id`,
		sai, body.Name, body.Region,
	).Scan(&id)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) handleListBuckets(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	rows, err := s.db.Query(r.Context(),
		`SELECT id, source_account_id, name, region, versioning, created_at
		 FROM np_storage_buckets WHERE source_account_id = $1 ORDER BY name`, sai)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var buckets []bucketRow
	for rows.Next() {
		var b bucketRow
		if err := rows.Scan(&b.ID, &b.SourceAccountID, &b.Name, &b.Region, &b.Versioning, &b.CreatedAt); err != nil {
			continue
		}
		buckets = append(buckets, b)
	}
	if buckets == nil {
		buckets = []bucketRow{}
	}
	writeJSON(w, http.StatusOK, buckets)
}

// handleDeleteBucket removes the account's tracked objects from S3, then the bucket row (cascade).
// The physical S3 bucket is shared across accounts and apps, so it is never deleted.
func (s *Server) handleDeleteBucket(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	bucket := chi.URLParam(r, "bucket")

	rows, err := s.db.Query(r.Context(),
		`SELECT o.key FROM np_storage_objects o
		 JOIN np_storage_buckets b ON b.id = o.bucket_id
		 WHERE b.source_account_id = $1 AND b.name = $2`, sai, bucket)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			rows.Close()
			http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
			return
		}
		keys = append(keys, objectKey(sai, k))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	if len(keys) > 0 {
		if s.store == nil {
			http.Error(w, errNoStore, http.StatusServiceUnavailable)
			return
		}
		if err := s.store.DeleteObjects(r.Context(), bucket, keys); err != nil {
			http.Error(w, errStoreFailed, http.StatusBadGateway)
			return
		}
	}

	_, err = s.db.Exec(r.Context(),
		`DELETE FROM np_storage_buckets WHERE source_account_id = $1 AND name = $2`, sai, bucket)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
