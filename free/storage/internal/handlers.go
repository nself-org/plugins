// Package internal provides the plugin-storage HTTP handlers.
//
// Purpose: S3-compatible storage handlers: bucket CRUD, object PUT/GET/DELETE/LIST.
// Inputs: HTTP requests with source_account_id from JWT header.
// Outputs: JSON responses or binary object content.
// Constraints:
//   - S3_ENDPOINT is config-only; never user-overridable per request.
//   - Object keys are validated to block path traversal.
//   - source_account_id from X-Hasura-Source-Account-Id header scopes all DB queries.
package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server holds handler dependencies.
type Server struct {
	db  *pgxpool.Pool
	cfg *Config
}

// NewServer creates a new Server instance.
func NewServer(db *pgxpool.Pool, cfg *Config) *Server {
	return &Server{db: db, cfg: cfg}
}

// Routes registers all storage routes on r.
func (s *Server) Routes(r chi.Router) {
	r.Get("/health", s.handleHealth)

	r.Route("/storage", func(r chi.Router) {
		// Bucket operations
		r.Post("/buckets", s.handleCreateBucket)
		r.Get("/buckets", s.handleListBuckets)
		r.Delete("/buckets/{bucket}", s.handleDeleteBucket)

		// Object operations
		r.Put("/buckets/{bucket}/objects/{key}", s.handlePutObject)
		r.Get("/buckets/{bucket}/objects/{key}", s.handleGetObject)
		r.Delete("/buckets/{bucket}/objects/{key}", s.handleDeleteObject)
		r.Get("/buckets/{bucket}/objects", s.handleListObjects)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		http.Error(w, `{"status":"unhealthy"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func sourceAccountID(r *http.Request) string {
	id := r.Header.Get("X-Hasura-Source-Account-Id")
	if id == "" {
		return "primary"
	}
	return id
}

// validateKey blocks path traversal in object keys.
// path.Clean resolves ".." so we compare the cleaned path against the key to detect traversal.
func validateKey(key string) error {
	if strings.Contains(key, "..") {
		return ErrInvalidKey
	}
	cleaned := path.Clean("/" + key)
	if strings.Contains(cleaned, "..") {
		return ErrInvalidKey
	}
	return nil
}

// ErrInvalidKey is returned when an object key fails validation.
var ErrInvalidKey = &storageError{"invalid object key: path traversal not allowed"}

type storageError struct{ msg string }

func (e *storageError) Error() string { return e.msg }

type bucketRow struct {
	ID               string    `json:"id"`
	SourceAccountID  string    `json:"source_account_id"`
	Name             string    `json:"name"`
	Region           string    `json:"region"`
	Versioning       bool      `json:"versioning"`
	CreatedAt        time.Time `json:"created_at"`
}

type objectRow struct {
	ID               string    `json:"id"`
	SourceAccountID  string    `json:"source_account_id"`
	BucketID         string    `json:"bucket_id"`
	Key              string    `json:"key"`
	SizeBytes        int64     `json:"size_bytes"`
	ContentType      string    `json:"content_type"`
	Etag             string    `json:"etag"`
	CreatedAt        time.Time `json:"created_at"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleCreateBucket(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	var body struct {
		Name    string `json:"name"`
		Region  string `json:"region"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if body.Region == "" {
		body.Region = "us-east-1"
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

func (s *Server) handleDeleteBucket(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	bucket := chi.URLParam(r, "bucket")
	_, err := s.db.Exec(r.Context(),
		`DELETE FROM np_storage_buckets WHERE source_account_id = $1 AND name = $2`, sai, bucket)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePutObject(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	bucket := chi.URLParam(r, "bucket")
	key := chi.URLParam(r, "key")

	if err := validateKey(key); err != nil {
		http.Error(w, `{"error":"invalid key"}`, http.StatusBadRequest)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Look up bucket ID
	var bucketID string
	err := s.db.QueryRow(r.Context(),
		`SELECT id FROM np_storage_buckets WHERE source_account_id = $1 AND name = $2`,
		sai, bucket).Scan(&bucketID)
	if err != nil {
		http.Error(w, `{"error":"bucket not found"}`, http.StatusNotFound)
		return
	}

	body := r.Body
	defer body.Close()
	// In production this would stream to S3-compatible backend.
	// Here we record the metadata in Postgres.
	var id string
	err = s.db.QueryRow(r.Context(),
		`INSERT INTO np_storage_objects (source_account_id, bucket_id, key, content_type)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (bucket_id, key) DO UPDATE SET content_type = EXCLUDED.content_type, updated_at = NOW()
		 RETURNING id`,
		sai, bucketID, key, contentType,
	).Scan(&id)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "key": key})
}

func (s *Server) handleGetObject(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	bucket := chi.URLParam(r, "bucket")
	key := chi.URLParam(r, "key")

	if err := validateKey(key); err != nil {
		http.Error(w, `{"error":"invalid key"}`, http.StatusBadRequest)
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
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, obj)
}

func (s *Server) handleDeleteObject(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	bucket := chi.URLParam(r, "bucket")
	key := chi.URLParam(r, "key")

	if err := validateKey(key); err != nil {
		http.Error(w, `{"error":"invalid key"}`, http.StatusBadRequest)
		return
	}

	_, err := s.db.Exec(r.Context(),
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
