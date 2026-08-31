package internal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handlers holds the database pool and implements all HTTP handlers.
type Handlers struct {
	pool *pgxpool.Pool
}

// NewHandlers returns a new Handlers instance.
func NewHandlers(pool *pgxpool.Pool) *Handlers {
	return &Handlers{pool: pool}
}

// sa extracts the source_account_id from request headers (multi-app isolation).
func (h *Handlers) sa(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account-ID"); v != "" {
		return v
	}
	return "primary"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b) //nolint:errcheck
	return hex.EncodeToString(b)
}

// renderTemplate performs simple {{key}} substitution on template content.
func renderTemplate(tmpl *Template, data map[string]any) string {
	html := tmpl.TemplateContent
	re := regexp.MustCompile(`\{\{\s*(\w+)\s*\}\}`)
	html = re.ReplaceAllStringFunc(html, func(match string) string {
		sub := re.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		if val, ok := data[sub[1]]; ok {
			return fmt.Sprintf("%v", val)
		}
		return ""
	})
	if tmpl.CSSContent != nil {
		html = "<style>" + *tmpl.CSSContent + "</style>" + html
	}
	if tmpl.HeaderContent != nil {
		html = *tmpl.HeaderContent + html
	}
	if tmpl.FooterContent != nil {
		html = html + *tmpl.FooterContent
	}
	return html
}

// =========================================================================
// Health
// =========================================================================

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"plugin":    "documents",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"ready": "false", "error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"ready": "true", "plugin": "documents",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// =========================================================================
// Documents — list + create
// =========================================================================

func (h *Handlers) ListDocuments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	q := r.URL.Query()

	conditions := []string{"source_account_id=$1"}
	args := []any{sa}
	idx := 2

	if v := q.Get("owner_id"); v != "" {
		conditions = append(conditions, fmt.Sprintf("owner_id=$%d", idx))
		args = append(args, v)
		idx++
	}
	if v := q.Get("doc_type"); v != "" {
		conditions = append(conditions, fmt.Sprintf("doc_type=$%d", idx))
		args = append(args, v)
		idx++
	}
	if v := q.Get("category"); v != "" {
		conditions = append(conditions, fmt.Sprintf("category=$%d", idx))
		args = append(args, v)
		idx++
	}
	if v := q.Get("status"); v != "" {
		conditions = append(conditions, fmt.Sprintf("status=$%d", idx))
		args = append(args, v)
		idx++
	}

	sql := "SELECT id,source_account_id,owner_id,title,description,doc_type,category,tags," +
		"template_id,file_url,file_size_bytes,mime_type,version,status,generated_from,metadata,created_at,updated_at " +
		"FROM docs_documents WHERE " + strings.Join(conditions, " AND ") +
		" ORDER BY updated_at DESC"

	// Default limit=50, cap at 1000 to prevent full table scans.
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}
	sql += fmt.Sprintf(" LIMIT $%d", idx)
	args = append(args, limit)
	idx++

	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			sql += fmt.Sprintf(" OFFSET $%d", idx)
			args = append(args, n)
		}
	}

	rows, err := h.pool.Query(ctx, sql, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	docs := []Document{}
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.SourceAccountID, &d.OwnerID, &d.Title, &d.Description,
			&d.DocType, &d.Category, &d.Tags, &d.TemplateID, &d.FileURL, &d.FileSizeBytes,
			&d.MimeType, &d.Version, &d.Status, &d.GeneratedFrom, &d.Metadata,
			&d.CreatedAt, &d.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		docs = append(docs, d)
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": docs, "count": len(docs)})
}

func (h *Handlers) CreateDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	var body struct {
		OwnerID       string         `json:"owner_id"`
		Title         string         `json:"title"`
		Description   *string        `json:"description"`
		DocType       string         `json:"doc_type"`
		Category      *string        `json:"category"`
		Tags          []string       `json:"tags"`
		FileURL       *string        `json:"file_url"`
		FileSizeBytes *int64         `json:"file_size_bytes"`
		MimeType      *string        `json:"mime_type"`
		Metadata      map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	tags := body.Tags
	if tags == nil {
		tags = []string{}
	}
	meta := "{}"
	if body.Metadata != nil {
		if b, err := json.Marshal(body.Metadata); err == nil {
			meta = string(b)
		}
	}

	var d Document
	err := h.pool.QueryRow(ctx,
		`INSERT INTO docs_documents
			(source_account_id,owner_id,title,description,doc_type,category,tags,
			 file_url,file_size_bytes,mime_type,version,status,metadata,
			 search_vector)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,1,'draft',$11,
			to_tsvector('english',$3||' '||COALESCE($4,'')||' '||$5))
		RETURNING id,source_account_id,owner_id,title,description,doc_type,category,tags,
			template_id,file_url,file_size_bytes,mime_type,version,status,generated_from,metadata,created_at,updated_at`,
		sa, body.OwnerID, body.Title, body.Description, body.DocType, body.Category,
		tags, body.FileURL, body.FileSizeBytes, body.MimeType, meta,
	).Scan(&d.ID, &d.SourceAccountID, &d.OwnerID, &d.Title, &d.Description,
		&d.DocType, &d.Category, &d.Tags, &d.TemplateID, &d.FileURL, &d.FileSizeBytes,
		&d.MimeType, &d.Version, &d.Status, &d.GeneratedFrom, &d.Metadata,
		&d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

// =========================================================================
// Documents — single record
// =========================================================================

func (h *Handlers) GetDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	var d Document
	err := h.pool.QueryRow(ctx,
		`SELECT id,source_account_id,owner_id,title,description,doc_type,category,tags,
			template_id,file_url,file_size_bytes,mime_type,version,status,generated_from,metadata,created_at,updated_at
		FROM docs_documents WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(&d.ID, &d.SourceAccountID, &d.OwnerID, &d.Title, &d.Description,
		&d.DocType, &d.Category, &d.Tags, &d.TemplateID, &d.FileURL, &d.FileSizeBytes,
		&d.MimeType, &d.Version, &d.Status, &d.GeneratedFrom, &d.Metadata,
		&d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handlers) UpdateDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	var body struct {
		Title       *string        `json:"title"`
		Description *string        `json:"description"`
		Category    *string        `json:"category"`
		Tags        []string       `json:"tags"`
		Status      *string        `json:"status"`
		Metadata    map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	sets := []string{"updated_at=NOW()"}
	args := []any{}
	idx := 1

	if body.Title != nil {
		sets = append(sets, fmt.Sprintf("title=$%d", idx))
		args = append(args, *body.Title)
		idx++
	}
	if body.Description != nil {
		sets = append(sets, fmt.Sprintf("description=$%d", idx))
		args = append(args, *body.Description)
		idx++
	}
	if body.Category != nil {
		sets = append(sets, fmt.Sprintf("category=$%d", idx))
		args = append(args, *body.Category)
		idx++
	}
	if body.Tags != nil {
		sets = append(sets, fmt.Sprintf("tags=$%d", idx))
		args = append(args, body.Tags)
		idx++
	}
	if body.Status != nil {
		sets = append(sets, fmt.Sprintf("status=$%d", idx))
		args = append(args, *body.Status)
		idx++
	}
	if body.Metadata != nil {
		b, _ := json.Marshal(body.Metadata)
		sets = append(sets, fmt.Sprintf("metadata=$%d::jsonb", idx))
		args = append(args, string(b))
		idx++
	}

	args = append(args, id, sa)
	var d Document
	err := h.pool.QueryRow(ctx,
		fmt.Sprintf(`UPDATE docs_documents SET %s WHERE id=$%d AND source_account_id=$%d
		RETURNING id,source_account_id,owner_id,title,description,doc_type,category,tags,
			template_id,file_url,file_size_bytes,mime_type,version,status,generated_from,metadata,created_at,updated_at`,
			strings.Join(sets, ","), idx, idx+1),
		args...,
	).Scan(&d.ID, &d.SourceAccountID, &d.OwnerID, &d.Title, &d.Description,
		&d.DocType, &d.Category, &d.Tags, &d.TemplateID, &d.FileURL, &d.FileSizeBytes,
		&d.MimeType, &d.Version, &d.Status, &d.GeneratedFrom, &d.Metadata,
		&d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handlers) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	tag, err := h.pool.Exec(ctx,
		`DELETE FROM docs_documents WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// =========================================================================
// Documents — actions
// =========================================================================

func (h *Handlers) PublishDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	tag, err := h.pool.Exec(ctx,
		`UPDATE docs_documents SET status='final', updated_at=NOW()
		WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "final"})
}

func (h *Handlers) ArchiveDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	tag, err := h.pool.Exec(ctx,
		`UPDATE docs_documents SET status='archived', updated_at=NOW()
		WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "archived"})
}

// =========================================================================
// Versions
// =========================================================================

func (h *Handlers) ListVersions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	docID := chi.URLParam(r, "id")

	// Verify document exists
	var exists bool
	h.pool.QueryRow(ctx, //nolint:errcheck
		`SELECT EXISTS(SELECT 1 FROM docs_documents WHERE id=$1 AND source_account_id=$2)`,
		docID, sa).Scan(&exists)
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}

	rows, err := h.pool.Query(ctx,
		`SELECT id,source_account_id,document_id,version,file_url,file_size_bytes,change_summary,created_by,created_at
		FROM docs_versions WHERE document_id=$1 AND source_account_id=$2 ORDER BY version DESC`,
		docID, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	versions := []DocVersion{}
	for rows.Next() {
		var v DocVersion
		if err := rows.Scan(&v.ID, &v.SourceAccountID, &v.DocumentID, &v.Version,
			&v.FileURL, &v.FileSizeBytes, &v.ChangeSummary, &v.CreatedBy, &v.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		versions = append(versions, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": versions, "count": len(versions)})
}

func (h *Handlers) CreateVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	docID := chi.URLParam(r, "id")

	var body struct {
		FileURL       string  `json:"file_url"`
		FileSizeBytes *int64  `json:"file_size_bytes"`
		ChangeSummary *string `json:"change_summary"`
		CreatedBy     *string `json:"created_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Get next version number
	var nextVer int
	h.pool.QueryRow(ctx, //nolint:errcheck
		`SELECT COALESCE(MAX(version),0)+1 FROM docs_versions WHERE document_id=$1 AND source_account_id=$2`,
		docID, sa).Scan(&nextVer)

	var v DocVersion
	err := h.pool.QueryRow(ctx,
		`INSERT INTO docs_versions (source_account_id,document_id,version,file_url,file_size_bytes,change_summary,created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id,source_account_id,document_id,version,file_url,file_size_bytes,change_summary,created_by,created_at`,
		sa, docID, nextVer, body.FileURL, body.FileSizeBytes, body.ChangeSummary, body.CreatedBy,
	).Scan(&v.ID, &v.SourceAccountID, &v.DocumentID, &v.Version,
		&v.FileURL, &v.FileSizeBytes, &v.ChangeSummary, &v.CreatedBy, &v.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

// =========================================================================
// Shares
// =========================================================================

func (h *Handlers) ListShares(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	docID := chi.URLParam(r, "id")

	rows, err := h.pool.Query(ctx,
		`SELECT id,source_account_id,document_id,shared_with_user_id,shared_with_email,
			share_token,permission,expires_at,accessed_at,created_at
		FROM docs_shares WHERE document_id=$1 AND source_account_id=$2 ORDER BY created_at DESC`,
		docID, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	shares := []Share{}
	for rows.Next() {
		var s Share
		if err := rows.Scan(&s.ID, &s.SourceAccountID, &s.DocumentID, &s.SharedWithUserID,
			&s.SharedWithEmail, &s.ShareToken, &s.Permission, &s.ExpiresAt,
			&s.AccessedAt, &s.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		shares = append(shares, s)
	}
	writeJSON(w, http.StatusOK, map[string]any{"shares": shares, "count": len(shares)})
}

func (h *Handlers) CreateShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	docID := chi.URLParam(r, "id")

	var exists bool
	h.pool.QueryRow(ctx, //nolint:errcheck
		`SELECT EXISTS(SELECT 1 FROM docs_documents WHERE id=$1 AND source_account_id=$2)`,
		docID, sa).Scan(&exists)
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}

	var body struct {
		SharedWithUserID *string `json:"shared_with_user_id"`
		SharedWithEmail  *string `json:"shared_with_email"`
		Permission       string  `json:"permission"`
		ExpiresAt        *string `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if body.Permission == "" {
		body.Permission = "view"
	}

	token := generateToken()

	var expiresAt *time.Time
	if body.ExpiresAt != nil {
		if t, err := time.Parse(time.RFC3339, *body.ExpiresAt); err == nil {
			expiresAt = &t
		}
	} else {
		t := time.Now().UTC().Add(30 * 24 * time.Hour)
		expiresAt = &t
	}

	var s Share
	err := h.pool.QueryRow(ctx,
		`INSERT INTO docs_shares
			(source_account_id,document_id,shared_with_user_id,shared_with_email,share_token,permission,expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id,source_account_id,document_id,shared_with_user_id,shared_with_email,
			share_token,permission,expires_at,accessed_at,created_at`,
		sa, docID, body.SharedWithUserID, body.SharedWithEmail, token, body.Permission, expiresAt,
	).Scan(&s.ID, &s.SourceAccountID, &s.DocumentID, &s.SharedWithUserID,
		&s.SharedWithEmail, &s.ShareToken, &s.Permission, &s.ExpiresAt,
		&s.AccessedAt, &s.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"share_id":  s.ID,
		"share_token": token,
		"share_url": "/api/v1/shared/" + token,
	})
}

func (h *Handlers) DeleteShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	tag, err := h.pool.Exec(ctx,
		`DELETE FROM docs_shares WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "share not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// =========================================================================
// Templates
// =========================================================================

func (h *Handlers) ListTemplates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	q := r.URL.Query()

	conditions := []string{"source_account_id=$1"}
	args := []any{sa}
	idx := 2

	if v := q.Get("doc_type"); v != "" {
		conditions = append(conditions, fmt.Sprintf("doc_type=$%d", idx))
		args = append(args, v)
		idx++
	}

	sql := "SELECT id,source_account_id,name,description,doc_type,output_format,template_engine," +
		"template_content,css_content,header_content,footer_content,variables,sample_data,is_default,version,created_at,updated_at " +
		"FROM docs_templates WHERE " + strings.Join(conditions, " AND ") +
		" ORDER BY name ASC, version DESC"

	// Default limit=50, cap at 1000 to prevent full table scans.
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}
	sql += fmt.Sprintf(" LIMIT $%d", idx)
	args = append(args, limit)
	idx++

	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			sql += fmt.Sprintf(" OFFSET $%d", idx)
			args = append(args, n)
		}
	}

	rows, err := h.pool.Query(ctx, sql, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	tmpls := []Template{}
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.SourceAccountID, &t.Name, &t.Description, &t.DocType,
			&t.OutputFormat, &t.TemplateEngine, &t.TemplateContent, &t.CSSContent,
			&t.HeaderContent, &t.FooterContent, &t.Variables, &t.SampleData,
			&t.IsDefault, &t.Version, &t.CreatedAt, &t.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		tmpls = append(tmpls, t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": tmpls, "count": len(tmpls)})
}

func (h *Handlers) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	var body struct {
		Name            string         `json:"name"`
		Description     *string        `json:"description"`
		DocType         string         `json:"doc_type"`
		OutputFormat    string         `json:"output_format"`
		TemplateEngine  string         `json:"template_engine"`
		TemplateContent string         `json:"template_content"`
		CSSContent      *string        `json:"css_content"`
		HeaderContent   *string        `json:"header_content"`
		FooterContent   *string        `json:"footer_content"`
		Variables       map[string]any `json:"variables"`
		SampleData      map[string]any `json:"sample_data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if body.OutputFormat == "" {
		body.OutputFormat = "pdf"
	}
	if body.TemplateEngine == "" {
		body.TemplateEngine = "handlebars"
	}
	vars := "{}"
	if body.Variables != nil {
		if b, err := json.Marshal(body.Variables); err == nil {
			vars = string(b)
		}
	}
	sample := "{}"
	if body.SampleData != nil {
		if b, err := json.Marshal(body.SampleData); err == nil {
			sample = string(b)
		}
	}

	var t Template
	err := h.pool.QueryRow(ctx,
		`INSERT INTO docs_templates
			(source_account_id,name,description,doc_type,output_format,template_engine,
			 template_content,css_content,header_content,footer_content,variables,sample_data,is_default,version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,false,1)
		RETURNING id,source_account_id,name,description,doc_type,output_format,template_engine,
			template_content,css_content,header_content,footer_content,variables,sample_data,is_default,version,created_at,updated_at`,
		sa, body.Name, body.Description, body.DocType, body.OutputFormat, body.TemplateEngine,
		body.TemplateContent, body.CSSContent, body.HeaderContent, body.FooterContent,
		vars, sample,
	).Scan(&t.ID, &t.SourceAccountID, &t.Name, &t.Description, &t.DocType,
		&t.OutputFormat, &t.TemplateEngine, &t.TemplateContent, &t.CSSContent,
		&t.HeaderContent, &t.FooterContent, &t.Variables, &t.SampleData,
		&t.IsDefault, &t.Version, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (h *Handlers) GetTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	var t Template
	err := h.pool.QueryRow(ctx,
		`SELECT id,source_account_id,name,description,doc_type,output_format,template_engine,
			template_content,css_content,header_content,footer_content,variables,sample_data,is_default,version,created_at,updated_at
		FROM docs_templates WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(&t.ID, &t.SourceAccountID, &t.Name, &t.Description, &t.DocType,
		&t.OutputFormat, &t.TemplateEngine, &t.TemplateContent, &t.CSSContent,
		&t.HeaderContent, &t.FooterContent, &t.Variables, &t.SampleData,
		&t.IsDefault, &t.Version, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "template not found"})
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handlers) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	var body struct {
		Name            *string        `json:"name"`
		Description     *string        `json:"description"`
		DocType         *string        `json:"doc_type"`
		OutputFormat    *string        `json:"output_format"`
		TemplateEngine  *string        `json:"template_engine"`
		TemplateContent *string        `json:"template_content"`
		CSSContent      *string        `json:"css_content"`
		HeaderContent   *string        `json:"header_content"`
		FooterContent   *string        `json:"footer_content"`
		Variables       map[string]any `json:"variables"`
		SampleData      map[string]any `json:"sample_data"`
		IsDefault       *bool          `json:"is_default"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	sets := []string{"updated_at=NOW()"}
	args := []any{}
	idx := 1

	strFields := map[string]*string{
		"name": body.Name, "description": body.Description, "doc_type": body.DocType,
		"output_format": body.OutputFormat, "template_engine": body.TemplateEngine,
		"template_content": body.TemplateContent, "css_content": body.CSSContent,
		"header_content": body.HeaderContent, "footer_content": body.FooterContent,
	}
	for col, val := range strFields {
		if val != nil {
			sets = append(sets, fmt.Sprintf("%s=$%d", col, idx))
			args = append(args, *val)
			idx++
		}
	}
	if body.IsDefault != nil {
		sets = append(sets, fmt.Sprintf("is_default=$%d", idx))
		args = append(args, *body.IsDefault)
		idx++
	}
	if body.Variables != nil {
		b, _ := json.Marshal(body.Variables)
		sets = append(sets, fmt.Sprintf("variables=$%d::jsonb", idx))
		args = append(args, string(b))
		idx++
	}
	if body.SampleData != nil {
		b, _ := json.Marshal(body.SampleData)
		sets = append(sets, fmt.Sprintf("sample_data=$%d::jsonb", idx))
		args = append(args, string(b))
		idx++
	}

	args = append(args, id, sa)
	var t Template
	err := h.pool.QueryRow(ctx,
		fmt.Sprintf(`UPDATE docs_templates SET %s WHERE id=$%d AND source_account_id=$%d
		RETURNING id,source_account_id,name,description,doc_type,output_format,template_engine,
			template_content,css_content,header_content,footer_content,variables,sample_data,is_default,version,created_at,updated_at`,
			strings.Join(sets, ","), idx, idx+1),
		args...,
	).Scan(&t.ID, &t.SourceAccountID, &t.Name, &t.Description, &t.DocType,
		&t.OutputFormat, &t.TemplateEngine, &t.TemplateContent, &t.CSSContent,
		&t.HeaderContent, &t.FooterContent, &t.Variables, &t.SampleData,
		&t.IsDefault, &t.Version, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "template not found"})
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handlers) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	tag, err := h.pool.Exec(ctx,
		`DELETE FROM docs_templates WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "template not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// =========================================================================
// Stats
// =========================================================================

func (h *Handlers) Stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	var totalDocs, totalTemplates, totalShares, totalVersions, recentDocs int64
	h.pool.QueryRow(ctx, //nolint:errcheck
		`SELECT
			(SELECT COUNT(*) FROM docs_documents      WHERE source_account_id=$1),
			(SELECT COUNT(*) FROM docs_templates      WHERE source_account_id=$1),
			(SELECT COUNT(*) FROM docs_shares         WHERE source_account_id=$1),
			(SELECT COUNT(*) FROM docs_versions       WHERE source_account_id=$1),
			(SELECT COUNT(*) FROM docs_documents      WHERE source_account_id=$1 AND created_at >= NOW()-INTERVAL '7 days')`,
		sa).Scan(&totalDocs, &totalTemplates, &totalShares, &totalVersions, &recentDocs)

	// By type
	typeRows, _ := h.pool.Query(ctx,
		`SELECT doc_type, COUNT(*) FROM docs_documents WHERE source_account_id=$1 GROUP BY doc_type`, sa)
	byType := map[string]int64{}
	if typeRows != nil {
		defer typeRows.Close()
		for typeRows.Next() {
			var dt string
			var cnt int64
			typeRows.Scan(&dt, &cnt) //nolint:errcheck
			byType[dt] = cnt
		}
	}

	// By category
	catRows, _ := h.pool.Query(ctx,
		`SELECT COALESCE(category,'uncategorized'), COUNT(*) FROM docs_documents WHERE source_account_id=$1 GROUP BY category`, sa)
	byCategory := map[string]int64{}
	if catRows != nil {
		defer catRows.Close()
		for catRows.Next() {
			var cat string
			var cnt int64
			catRows.Scan(&cat, &cnt) //nolint:errcheck
			byCategory[cat] = cnt
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"plugin":    "documents",
		"version":   "1.0.0",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"stats": map[string]any{
			"total_documents":  totalDocs,
			"total_templates":  totalTemplates,
			"total_shares":     totalShares,
			"total_versions":   totalVersions,
			"recent_documents": recentDocs,
			"by_type":          byType,
			"by_category":      byCategory,
		},
	})
}
