package internal

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/nself-org/plugin-storage/internal/s3store"
)

// handlePutObject streams the request body into S3 (multipart above 16 MiB), then records the metadata row.
// The bytes never touch Postgres.
func (s *Server) handlePutObject(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	bucket := chi.URLParam(r, "bucket")
	key, keyOK := objectKeyParam(r)

	if err := validateObject(sai, key); !keyOK || err != nil {
		http.Error(w, `{"error":"invalid key"}`, http.StatusBadRequest)
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if !validContentType(contentType) {
		http.Error(w, `{"error":"invalid content type"}`, http.StatusBadRequest)
		return
	}
	if s.store == nil {
		http.Error(w, errNoStore, http.StatusServiceUnavailable)
		return
	}

	var bucketID string
	var existed bool // a row for this key exists already: an overwrite keeps its bytes if the row write fails
	err := s.db.QueryRow(r.Context(),
		`SELECT b.id, EXISTS (SELECT 1 FROM np_storage_objects o WHERE o.bucket_id = b.id AND o.key = $3)
		 FROM np_storage_buckets b WHERE b.source_account_id = $1 AND b.name = $2`,
		sai, bucket, key).Scan(&bucketID, &existed)
	if err != nil {
		writeLookupError(w, err, `{"error":"bucket not found"}`, "put "+bucket+"/"+key+": bucket lookup")
		return
	}

	defer r.Body.Close()
	body := newIdleReader(w, r.Body, uploadIdleTimeout)
	res, err := s.store.Put(r.Context(), bucket, objectKey(sai, key), body, contentType, r.ContentLength)
	if err != nil {
		log.Printf("storage: put %s/%s: %v", bucket, key, err)
		writePutError(w, err)
		return
	}
	// The body is in: the server's own write window may have run out during a long upload.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(downloadWriteTimeout))

	var id string
	err = s.db.QueryRow(r.Context(),
		`INSERT INTO np_storage_objects (source_account_id, bucket_id, key, size_bytes, content_type, etag)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (bucket_id, key) DO UPDATE
		   SET size_bytes = EXCLUDED.size_bytes, content_type = EXCLUDED.content_type,
		       etag = EXCLUDED.etag, updated_at = NOW()
		 RETURNING id`,
		sai, bucketID, key, res.Size, contentType, res.ETag,
	).Scan(&id)
	if err != nil {
		log.Printf("storage: put %s/%s: metadata upsert failed after the S3 write: %v", bucket, key, err)
		if !existed {
			s.removeOrphan(bucket, objectKey(sai, key))
		}
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "key": key, "size_bytes": res.Size, "etag": res.ETag})
}

// handleGetObject streams the object bytes from S3. With ?metadata=true it returns the metadata row as JSON
// (the pre-S3 behaviour, kept for clients that read it).
func (s *Server) handleGetObject(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	bucket := chi.URLParam(r, "bucket")
	key, keyOK := objectKeyParam(r)

	if err := validateObject(sai, key); !keyOK || err != nil {
		http.Error(w, `{"error":"invalid key"}`, http.StatusBadRequest)
		return
	}
	wantMeta := r.URL.Query().Get("metadata") == "true"
	if !wantMeta && s.store == nil {
		http.Error(w, errNoStore, http.StatusServiceUnavailable)
		return
	}

	var obj objectRow
	err := s.db.QueryRow(r.Context(),
		`SELECT o.id, o.source_account_id, o.bucket_id, o.key, o.size_bytes, o.content_type, COALESCE(o.etag,''), o.created_at
		 FROM np_storage_objects o
		 JOIN np_storage_buckets b ON b.id = o.bucket_id
		 WHERE b.source_account_id = $1 AND b.name = $2 AND o.key = $3`,
		sai, bucket, key,
	).Scan(&obj.ID, &obj.SourceAccountID, &obj.BucketID, &obj.Key, &obj.SizeBytes, &obj.ContentType, &obj.Etag, &obj.CreatedAt)
	if err != nil {
		writeLookupError(w, err, `{"error":"not found"}`, "get "+bucket+"/"+key+": row lookup")
		return
	}
	if wantMeta {
		writeJSON(w, http.StatusOK, obj)
		return
	}

	o, err := s.store.Get(r.Context(), bucket, objectKey(sai, key))
	if err != nil {
		if errors.Is(err, s3store.ErrNotFound) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		log.Printf("storage: get %s/%s: %v", bucket, key, err)
		http.Error(w, errStoreFailed, http.StatusBadGateway)
		return
	}
	defer o.Body.Close()
	// The Content-Type stored with the bytes wins over the row: concurrent PUTs of one key can leave the row stale.
	ct := o.ContentType
	if ct == "" {
		ct = obj.ContentType
	}
	setObjectHeaders(w, ct, o.Size, o.ETag)
	w.WriteHeader(http.StatusOK)
	streamBody(w, o.Body, "get "+bucket+"/"+key)
}

// setObjectHeaders sets the response headers of an object download. Stored HTML/SVG must not run script from the
// plugin's origin, hence nosniff and a sandboxing CSP. Content-Length is omitted when S3 did not report a size.
func setObjectHeaders(w http.ResponseWriter, contentType string, size int64, etag string) {
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox")
	if size >= 0 {
		h.Set("Content-Length", strconv.FormatInt(size, 10))
	}
	if etag != "" {
		h.Set("ETag", etag)
	}
}

// handleDeleteObject deletes the object from S3, then its metadata row. An unknown bucket answers 204.
func (s *Server) handleDeleteObject(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	bucket := chi.URLParam(r, "bucket")
	key, keyOK := objectKeyParam(r)

	if err := validateObject(sai, key); !keyOK || err != nil {
		http.Error(w, `{"error":"invalid key"}`, http.StatusBadRequest)
		return
	}

	var known bool
	err := s.db.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM np_storage_buckets WHERE source_account_id = $1 AND name = $2)`,
		sai, bucket).Scan(&known)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	if !known {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if s.store == nil {
		http.Error(w, errNoStore, http.StatusServiceUnavailable)
		return
	}
	// A key (or whole physical bucket) that is already gone counts as deleted.
	if err := s.store.Delete(r.Context(), bucket, objectKey(sai, key)); err != nil && !errors.Is(err, s3store.ErrNotFound) {
		log.Printf("storage: delete %s/%s: %v", bucket, key, err)
		http.Error(w, errStoreFailed, http.StatusBadGateway)
		return
	}

	_, err = s.db.Exec(r.Context(),
		`DELETE FROM np_storage_objects o
		 USING np_storage_buckets b
		 WHERE b.id = o.bucket_id AND b.source_account_id = $1 AND b.name = $2 AND o.key = $3`,
		sai, bucket, key)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListObjects(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	bucket := chi.URLParam(r, "bucket")
	prefix := r.URL.Query().Get("prefix")

	rows, err := s.db.Query(r.Context(),
		`SELECT o.id, o.source_account_id, o.bucket_id, o.key, o.size_bytes, o.content_type, COALESCE(o.etag,''), o.created_at
		 FROM np_storage_objects o
		 JOIN np_storage_buckets b ON b.id = o.bucket_id
		 WHERE b.source_account_id = $1 AND b.name = $2 AND o.key LIKE $3
		 ORDER BY o.key`,
		sai, bucket, prefix+"%")
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var objects []objectRow
	for rows.Next() {
		var o objectRow
		if err := rows.Scan(&o.ID, &o.SourceAccountID, &o.BucketID, &o.Key, &o.SizeBytes, &o.ContentType, &o.Etag, &o.CreatedAt); err != nil {
			continue
		}
		objects = append(objects, o)
	}
	if objects == nil {
		objects = []objectRow{}
	}
	writeJSON(w, http.StatusOK, objects)
}

// removeOrphan deletes bytes that were written to S3 but never got a metadata row (best effort, detached from the
// request so a client that hung up does not leave them behind). An overwrite is never cleaned up this way.
func (s *Server) removeOrphan(bucket, physicalKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.store.Delete(ctx, bucket, physicalKey); err != nil {
		log.Printf("storage: orphan cleanup of %s/%s failed: %v", bucket, physicalKey, err)
		return
	}
	log.Printf("storage: orphan cleanup removed %s/%s (metadata row was not written)", bucket, physicalKey)
}
