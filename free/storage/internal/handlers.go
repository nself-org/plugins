// Package internal provides the plugin-storage HTTP handlers.
//
// Purpose: storage handlers: bucket CRUD, object PUT/GET/DELETE/LIST. Object bytes live in the configured
// S3 endpoint (see objectstore.go); Postgres holds only metadata rows.
// Inputs: HTTP requests with source_account_id from JWT header.
// Outputs: JSON responses or binary object content.
// Constraints:
//   - S3_ENDPOINT is config-only; never user-overridable per request.
//   - Object bytes never touch Postgres; with no S3 credentials configured object routes answer 503.
//   - Object keys are validated to block path traversal.
//   - source_account_id from X-Hasura-Source-Account-Id header scopes all DB queries.
package internal

import (
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
	db    *pgxpool.Pool
	cfg   *Config
	store ObjectStore // nil when S3 is not configured
}

// NewServer creates a new Server instance. A nil store means S3 is not configured: object routes answer 503.
// Callers holding a *s3store.Store must check it for nil before passing it (a nil pointer in the interface is not nil).
func NewServer(db *pgxpool.Pool, cfg *Config, store ObjectStore) *Server {
	return &Server{db: db, cfg: cfg, store: store}
}

// Routes registers all storage routes on r.
func (s *Server) Routes(r chi.Router) {
	r.Get("/health", s.handleHealth)

	r.Route("/storage", func(r chi.Router) {
		r.Use(requireValidAccount)
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

// validAccountID reports whether a source account id may become the first segment of a physical S3 key:
// not blank, not "." or "..", no control characters.
func validAccountID(id string) bool {
	if strings.TrimSpace(id) == "" || id == "." || id == ".." {
		return false
	}
	for _, c := range id {
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}

// requireValidAccount answers 400 when the account header is present but unusable. An absent or empty header
// still means the default account "primary".
func requireValidAccount(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := r.Header.Get("X-Hasura-Source-Account-Id"); id != "" && !validAccountID(id) {
			http.Error(w, `{"error":"invalid source account id"}`, http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sourceAccountID(r *http.Request) string {
	id := r.Header.Get("X-Hasura-Source-Account-Id")
	if id == "" {
		return "primary"
	}
	return id
}

// maxKeyBytes is the S3 object key length limit.
const maxKeyBytes = 1024

// validateKey blocks path traversal in object keys and rejects empty, over-long and control-character keys.
// path.Clean resolves ".." so we compare the cleaned path against the key to detect traversal.
func validateKey(key string) error {
	if key == "" || len(key) > maxKeyBytes || key == "." || strings.HasSuffix(key, "/.") {
		return ErrInvalidKey
	}
	for i := 0; i < len(key); i++ {
		if key[i] < 0x20 || key[i] == 0x7f { // no newline in logs or in an S3 key
			return ErrInvalidKey
		}
	}
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
var ErrInvalidKey = &storageError{"invalid object key: empty, too long, or path traversal"}

type storageError struct{ msg string }

func (e *storageError) Error() string { return e.msg }

type bucketRow struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	Name            string    `json:"name"`
	Region          string    `json:"region"`
	Versioning      bool      `json:"versioning"`
	CreatedAt       time.Time `json:"created_at"`
}

type objectRow struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	BucketID        string    `json:"bucket_id"`
	Key             string    `json:"key"`
	SizeBytes       int64     `json:"size_bytes"`
	ContentType     string    `json:"content_type"`
	Etag            string    `json:"etag"`
	CreatedAt       time.Time `json:"created_at"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// errNoStore is the 503 body for every route that needs S3 while it is not configured.
const errNoStore = `{"error":"object storage not configured"}`

// errStoreFailed is the 502 body when the S3 endpoint fails an operation.
const errStoreFailed = `{"error":"object storage error"}`
