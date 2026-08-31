package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler holds the DB pool and config used by all route handlers.
type Handler struct {
	pool *pgxpool.Pool
	cfg  *Config
}

// NewHandler creates a Handler with the given pool and config.
func NewHandler(pool *pgxpool.Pool, cfg *Config) *Handler {
	return &Handler{pool: pool, cfg: cfg}
}

// sa returns the source_account_id from the X-Source-Account header, defaulting to "primary".
func sa(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account"); v != "" {
		return v
	}
	return "primary"
}

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// errJSON writes a {"error": msg} response.
func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ============================================================
// Buckets
// ============================================================

// ListBuckets handles GET /api/v1/buckets
func (h *Handler) ListBuckets(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, name, provider, provider_config,
		        public_read, cors_origins, max_file_size_bytes, allowed_mime_types,
		        quota_bytes, used_bytes, object_count, lifecycle_rules,
		        created_at, updated_at
		   FROM os_buckets
		  WHERE source_account_id = $1
		  ORDER BY created_at DESC`, account)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	buckets := []Bucket{}
	for rows.Next() {
		var b Bucket
		var providerConfigRaw []byte
		var lifecycleRaw []byte
		if err := rows.Scan(
			&b.ID, &b.SourceAccountID, &b.Name, &b.Provider, &providerConfigRaw,
			&b.PublicRead, &b.CORSOrigins, &b.MaxFileSizeBytes, &b.AllowedMIMETypes,
			&b.QuotaBytes, &b.UsedBytes, &b.ObjectCount, &lifecycleRaw,
			&b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = json.Unmarshal(providerConfigRaw, &b.ProviderConfig)
		_ = json.Unmarshal(lifecycleRaw, &b.LifecycleRules)
		buckets = append(buckets, b)
	}
	writeJSON(w, http.StatusOK, buckets)
}

// CreateBucket handles POST /api/v1/buckets
func (h *Handler) CreateBucket(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	var body struct {
		Name             string         `json:"name"`
		Provider         string         `json:"provider"`
		ProviderConfig   map[string]any `json:"provider_config"`
		PublicRead       bool           `json:"public_read"`
		CORSOrigins      []string       `json:"cors_origins"`
		MaxFileSizeBytes int64          `json:"max_file_size_bytes"`
		AllowedMIMETypes []string       `json:"allowed_mime_types"`
		QuotaBytes       *int64         `json:"quota_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Name == "" {
		errJSON(w, http.StatusBadRequest, "name is required")
		return
	}
	if body.Provider == "" {
		body.Provider = h.cfg.DefaultProvider
	}
	if body.MaxFileSizeBytes == 0 {
		body.MaxFileSizeBytes = 104857600
	}

	pcJSON, _ := json.Marshal(body.ProviderConfig)
	corsOrigins := body.CORSOrigins
	if corsOrigins == nil {
		corsOrigins = []string{}
	}
	mimeTypes := body.AllowedMIMETypes
	if mimeTypes == nil {
		mimeTypes = []string{}
	}

	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO os_buckets
		        (source_account_id, name, provider, provider_config, public_read,
		         cors_origins, max_file_size_bytes, allowed_mime_types, quota_bytes,
		         used_bytes, object_count, lifecycle_rules)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,0,0,'[]')
		 RETURNING id`,
		account, body.Name, body.Provider, pcJSON, body.PublicRead,
		corsOrigins, body.MaxFileSizeBytes, mimeTypes, body.QuotaBytes,
	).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// GetBucket handles GET /api/v1/buckets/{id}
func (h *Handler) GetBucket(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)

	var b Bucket
	var providerConfigRaw, lifecycleRaw []byte
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, name, provider, provider_config,
		        public_read, cors_origins, max_file_size_bytes, allowed_mime_types,
		        quota_bytes, used_bytes, object_count, lifecycle_rules,
		        created_at, updated_at
		   FROM os_buckets WHERE id = $1 AND source_account_id = $2`, id, account,
	).Scan(
		&b.ID, &b.SourceAccountID, &b.Name, &b.Provider, &providerConfigRaw,
		&b.PublicRead, &b.CORSOrigins, &b.MaxFileSizeBytes, &b.AllowedMIMETypes,
		&b.QuotaBytes, &b.UsedBytes, &b.ObjectCount, &lifecycleRaw,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		errJSON(w, http.StatusNotFound, "bucket not found")
		return
	}
	_ = json.Unmarshal(providerConfigRaw, &b.ProviderConfig)
	_ = json.Unmarshal(lifecycleRaw, &b.LifecycleRules)
	writeJSON(w, http.StatusOK, b)
}

// UpdateBucket handles PUT /api/v1/buckets/{id}
func (h *Handler) UpdateBucket(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)

	var body struct {
		PublicRead       *bool    `json:"public_read"`
		CORSOrigins      []string `json:"cors_origins"`
		MaxFileSizeBytes *int64   `json:"max_file_size_bytes"`
		AllowedMIMETypes []string `json:"allowed_mime_types"`
		QuotaBytes       *int64   `json:"quota_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	_, err := h.pool.Exec(r.Context(),
		`UPDATE os_buckets SET
		   public_read = COALESCE($3, public_read),
		   cors_origins = COALESCE($4, cors_origins),
		   max_file_size_bytes = COALESCE($5, max_file_size_bytes),
		   allowed_mime_types = COALESCE($6, allowed_mime_types),
		   quota_bytes = $7,
		   updated_at = NOW()
		 WHERE id = $1 AND source_account_id = $2`,
		id, account,
		body.PublicRead, body.CORSOrigins, body.MaxFileSizeBytes, body.AllowedMIMETypes,
		body.QuotaBytes,
	)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

// DeleteBucket handles DELETE /api/v1/buckets/{id}
func (h *Handler) DeleteBucket(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	_, err := h.pool.Exec(r.Context(),
		`DELETE FROM os_buckets WHERE id = $1 AND source_account_id = $2`, id, account)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Objects
// ============================================================

// ListObjects handles GET /api/v1/buckets/{id}/objects
func (h *Handler) ListObjects(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "id")
	account := sa(r)
	q := r.URL.Query()
	prefix := q.Get("prefix")
	maxKeys := 1000
	if mk := q.Get("max_keys"); mk != "" {
		if v, err := strconv.Atoi(mk); err == nil && v > 0 {
			maxKeys = v
		}
	}

	query := `SELECT id, source_account_id, bucket_id, key, filename, content_type,
	                  size_bytes, checksum_sha256, etag, storage_class, metadata, tags,
	                  owner_id, is_public, version, deleted_at, created_at, updated_at
	            FROM os_objects
	           WHERE source_account_id = $1 AND bucket_id = $2 AND deleted_at IS NULL`
	args := []any{account, bucketID}

	if prefix != "" {
		args = append(args, prefix+"%")
		query += ` AND key LIKE $` + strconv.Itoa(len(args))
	}
	args = append(args, maxKeys)
	query += ` ORDER BY key ASC LIMIT $` + strconv.Itoa(len(args))

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	objects := []Object{}
	for rows.Next() {
		var o Object
		var metaRaw, tagsRaw []byte
		if err := rows.Scan(
			&o.ID, &o.SourceAccountID, &o.BucketID, &o.Key, &o.Filename,
			&o.ContentType, &o.SizeBytes, &o.ChecksumSHA256, &o.ETag,
			&o.StorageClass, &metaRaw, &tagsRaw, &o.OwnerID, &o.IsPublic,
			&o.Version, &o.DeletedAt, &o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = json.Unmarshal(metaRaw, &o.Metadata)
		_ = json.Unmarshal(tagsRaw, &o.Tags)
		objects = append(objects, o)
	}
	writeJSON(w, http.StatusOK, objects)
}

// CreateObject handles POST /api/v1/buckets/{id}/objects
func (h *Handler) CreateObject(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "id")
	account := sa(r)

	var body struct {
		Key         string         `json:"key"`
		ContentType string         `json:"content_type"`
		SizeBytes   int64          `json:"size_bytes"`
		ETag        *string        `json:"etag"`
		Metadata    map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Key == "" {
		errJSON(w, http.StatusBadRequest, "key is required")
		return
	}

	metaJSON, _ := json.Marshal(body.Metadata)

	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO os_objects
		        (source_account_id, bucket_id, key, content_type, size_bytes, etag,
		         storage_class, metadata, tags, is_public, version)
		 VALUES ($1,$2,$3,$4,$5,$6,'standard',$7,'{}',false,1)
		 RETURNING id`,
		account, bucketID, body.Key, body.ContentType, body.SizeBytes, body.ETag, metaJSON,
	).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Update bucket counters.
	_, _ = h.pool.Exec(r.Context(),
		`UPDATE os_buckets SET used_bytes = used_bytes + $1, object_count = object_count + 1, updated_at = NOW()
		  WHERE id = $2 AND source_account_id = $3`,
		body.SizeBytes, bucketID, account)

	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// GetObject handles GET /api/v1/buckets/{id}/objects/{key}
func (h *Handler) GetObject(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")
	account := sa(r)

	var o Object
	var metaRaw, tagsRaw []byte
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, bucket_id, key, filename, content_type,
		        size_bytes, checksum_sha256, etag, storage_class, metadata, tags,
		        owner_id, is_public, version, deleted_at, created_at, updated_at
		   FROM os_objects
		  WHERE source_account_id = $1 AND bucket_id = $2 AND key = $3 AND deleted_at IS NULL
		  ORDER BY version DESC LIMIT 1`, account, bucketID, key,
	).Scan(
		&o.ID, &o.SourceAccountID, &o.BucketID, &o.Key, &o.Filename,
		&o.ContentType, &o.SizeBytes, &o.ChecksumSHA256, &o.ETag,
		&o.StorageClass, &metaRaw, &tagsRaw, &o.OwnerID, &o.IsPublic,
		&o.Version, &o.DeletedAt, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		errJSON(w, http.StatusNotFound, "object not found")
		return
	}
	_ = json.Unmarshal(metaRaw, &o.Metadata)
	_ = json.Unmarshal(tagsRaw, &o.Tags)
	writeJSON(w, http.StatusOK, o)
}

// DeleteObject handles DELETE /api/v1/buckets/{id}/objects/{key}
func (h *Handler) DeleteObject(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")
	account := sa(r)

	var sizeBytes int64
	var objID string
	err := h.pool.QueryRow(r.Context(),
		`UPDATE os_objects SET deleted_at = NOW()
		  WHERE source_account_id = $1 AND bucket_id = $2 AND key = $3 AND deleted_at IS NULL
		  RETURNING id, size_bytes`, account, bucketID, key,
	).Scan(&objID, &sizeBytes)
	if err != nil {
		errJSON(w, http.StatusNotFound, "object not found")
		return
	}

	_, _ = h.pool.Exec(r.Context(),
		`UPDATE os_buckets SET used_bytes = GREATEST(0, used_bytes - $1), object_count = GREATEST(0, object_count - 1), updated_at = NOW()
		  WHERE id = $2 AND source_account_id = $3`,
		sizeBytes, bucketID, account)

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Upload Sessions
// ============================================================

// CreateUploadSession handles POST /api/v1/upload-sessions
func (h *Handler) CreateUploadSession(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	var body struct {
		BucketID      string  `json:"bucket_id"`
		Key           string  `json:"key"`
		ContentType   *string `json:"content_type"`
		TotalSizeBytes *int64 `json:"total_size_bytes"`
		UploadType    string  `json:"upload_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.BucketID == "" || body.Key == "" {
		errJSON(w, http.StatusBadRequest, "bucket_id and key are required")
		return
	}
	if body.UploadType == "" {
		body.UploadType = "direct"
	}

	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO os_upload_sessions
		        (source_account_id, bucket_id, key, content_type, total_size_bytes, upload_type, status)
		 VALUES ($1,$2,$3,$4,$5,$6,'initiated')
		 RETURNING id`,
		account, body.BucketID, body.Key, body.ContentType, body.TotalSizeBytes, body.UploadType,
	).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// UpdateUploadSession handles PUT /api/v1/upload-sessions/{id}
func (h *Handler) UpdateUploadSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)

	var body struct {
		Status         *string `json:"status"`
		PartsCompleted *int    `json:"parts_completed"`
		PartsTotal     *int    `json:"parts_total"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	_, err := h.pool.Exec(r.Context(),
		`UPDATE os_upload_sessions SET
		   status = COALESCE($3, status),
		   parts_completed = COALESCE($4, parts_completed),
		   parts_total = COALESCE($5, parts_total),
		   updated_at = NOW()
		 WHERE id = $1 AND source_account_id = $2`,
		id, account, body.Status, body.PartsCompleted, body.PartsTotal,
	)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

// DeleteUploadSession handles DELETE /api/v1/upload-sessions/{id} (abort)
func (h *Handler) DeleteUploadSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	_, err := h.pool.Exec(r.Context(),
		`UPDATE os_upload_sessions SET status = 'aborted', updated_at = NOW()
		  WHERE id = $1 AND source_account_id = $2`, id, account)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Access Logs
// ============================================================

// ListAccessLogs handles GET /api/v1/access-logs
func (h *Handler) ListAccessLogs(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	q := r.URL.Query()

	query := `SELECT id, source_account_id, bucket_id, object_id, action, actor_id,
	                  ip_address, user_agent, status, response_time_ms, bytes_transferred, created_at
	            FROM os_access_logs
	           WHERE source_account_id = $1`
	args := []any{account}

	if bucketID := q.Get("bucket_id"); bucketID != "" {
		args = append(args, bucketID)
		query += ` AND bucket_id = $` + strconv.Itoa(len(args))
	}
	if startDate := q.Get("start_date"); startDate != "" {
		if t, err := time.Parse(time.RFC3339, startDate); err == nil {
			args = append(args, t)
			query += ` AND created_at >= $` + strconv.Itoa(len(args))
		}
	}
	if endDate := q.Get("end_date"); endDate != "" {
		if t, err := time.Parse(time.RFC3339, endDate); err == nil {
			args = append(args, t)
			query += ` AND created_at <= $` + strconv.Itoa(len(args))
		}
	}

	query += ` ORDER BY created_at DESC LIMIT 1000`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	logs := []AccessLog{}
	for rows.Next() {
		var l AccessLog
		if err := rows.Scan(
			&l.ID, &l.SourceAccountID, &l.BucketID, &l.ObjectID, &l.Action,
			&l.ActorID, &l.IPAddress, &l.UserAgent, &l.Status, &l.ResponseTimeMs,
			&l.BytesTransferred, &l.CreatedAt,
		); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		logs = append(logs, l)
	}
	writeJSON(w, http.StatusOK, logs)
}

// ============================================================
// Stats
// ============================================================

// GetStats handles GET /api/v1/stats
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	account := sa(r)

	var totalBuckets, totalObjects int
	var totalBytes int64
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(DISTINCT id), COALESCE(SUM(object_count),0), COALESCE(SUM(used_bytes),0)
		   FROM os_buckets WHERE source_account_id = $1`, account,
	).Scan(&totalBuckets, &totalObjects, &totalBytes)

	writeJSON(w, http.StatusOK, map[string]any{
		"total_buckets": totalBuckets,
		"total_objects": totalObjects,
		"total_bytes":   totalBytes,
	})
}

// HealthCheck handles GET /health
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		errJSON(w, http.StatusServiceUnavailable, "database unreachable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
