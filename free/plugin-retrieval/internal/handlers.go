// Package internal provides the HTTP handlers for plugin-retrieval.
//
// Purpose: Expose /search (hybrid pgvector ANN + tsvector BM25 + RRF merge)
//   and /index (upsert document with embedding) endpoints.
// Inputs: HTTP JSON requests with source_account_id for multi-app isolation.
// Outputs: JSON search results sorted by RRF score.
// Constraints:
//   - SSRF: N/A — all queries go to local Postgres only, no outbound HTTP.
//   - source_account_id is enforced on all queries (Multi-App Isolation Convention).
//   - pgvector ANN uses <=> (cosine distance); results converted to similarity = 1 - dist.
//   - tsvector uses ts_rank_cd with plainto_tsquery; weights tuned for plugin content.
//   - Embedding is caller-provided (float32 slice); plugin does NOT call any AI provider.
package internal

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// SearchRequest is the payload for POST /search.
type SearchRequest struct {
	Query           string    `json:"query"`
	Embedding       []float32 `json:"embedding"`       // caller-provided vector (nullable)
	TopK            int       `json:"top_k"`           // max results (default 10)
	SourceAccountID string    `json:"source_account_id"`
}

// IndexRequest is the payload for POST /index.
type IndexRequest struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Content         string    `json:"content"`
	Embedding       []float32 `json:"embedding"`
	SourceAccountID string    `json:"source_account_id"`
}

// Handlers holds the database handle for request handlers.
type Handlers struct {
	DB *sql.DB
}

// Health returns 200 OK when the DB is reachable, 503 otherwise.
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"unhealthy"}`))
		return
	}
	if err := h.DB.PingContext(r.Context()); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"unhealthy"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// Search handles POST /search. Runs pgvector ANN and/or tsvector BM25 and merges with RRF.
//   If embedding is provided, runs vector search. If query is non-empty, runs text search.
//   Both require source_account_id for tenant isolation.
func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	if req.SourceAccountID == "" {
		req.SourceAccountID = "primary"
	}
	topK := req.TopK
	if topK <= 0 {
		topK = 10
	}

	var vectorResults []DocumentResult
	var textResults []DocumentResult

	// Vector search (pgvector ANN cosine distance)
	if len(req.Embedding) > 0 {
		var err error
		vectorResults, err = h.vectorSearch(r, req.SourceAccountID, req.Embedding, topK*2)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
	}

	// Full-text search (tsvector ts_rank)
	if strings.TrimSpace(req.Query) != "" {
		var err error
		textResults, err = h.textSearch(r, req.SourceAccountID, req.Query, topK*2)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
	}

	merged := RRFMerge(vectorResults, textResults, topK)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"results": merged,
		"count":   len(merged),
	})
}

// Index handles POST /index. Upserts a document and its embedding.
func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	var req IndexRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	if req.ID == "" || req.Content == "" {
		http.Error(w, `{"error":"id and content required"}`, http.StatusBadRequest)
		return
	}
	if req.SourceAccountID == "" {
		req.SourceAccountID = "primary"
	}

	_, err := h.DB.ExecContext(r.Context(), `
		INSERT INTO np_retrieval_documents (id, source_account_id, title, content)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id, source_account_id) DO UPDATE SET
			title = EXCLUDED.title,
			content = EXCLUDED.content,
			updated_at = NOW()
	`, req.ID, req.SourceAccountID, req.Title, req.Content)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if len(req.Embedding) > 0 {
		vecStr := float32SliceToPgVector(req.Embedding)
		_, err = h.DB.ExecContext(r.Context(), `
			INSERT INTO np_retrieval_embeddings (document_id, source_account_id, embedding)
			VALUES ($1, $2, $3::vector)
			ON CONFLICT (document_id, source_account_id) DO UPDATE SET
				embedding = EXCLUDED.embedding,
				updated_at = NOW()
		`, req.ID, req.SourceAccountID, vecStr)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"status":"indexed"}`))
}

// vectorSearch runs pgvector cosine ANN over np_retrieval_embeddings.
func (h *Handlers) vectorSearch(r *http.Request, accountID string, embedding []float32, limit int) ([]DocumentResult, error) {
	vecStr := float32SliceToPgVector(embedding)
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT e.document_id, d.title, d.content,
		       1 - (e.embedding <=> $1::vector) AS similarity
		FROM np_retrieval_embeddings e
		JOIN np_retrieval_documents d ON d.id = e.document_id AND d.source_account_id = e.source_account_id
		WHERE e.source_account_id = $2
		ORDER BY e.embedding <=> $1::vector
		LIMIT $3
	`, vecStr, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DocumentResult
	for rows.Next() {
		var doc DocumentResult
		if err := rows.Scan(&doc.ID, &doc.Title, &doc.Content, &doc.Similarity); err != nil {
			return nil, err
		}
		doc.SourceAccountID = accountID
		results = append(results, doc)
	}
	return results, rows.Err()
}

// textSearch runs tsvector ts_rank_cd full-text search over np_retrieval_documents.
func (h *Handlers) textSearch(r *http.Request, accountID, query string, limit int) ([]DocumentResult, error) {
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT id, title, content,
		       ts_rank_cd(to_tsvector('english', content), plainto_tsquery('english', $1)) AS rank
		FROM np_retrieval_documents
		WHERE source_account_id = $2
		  AND to_tsvector('english', content) @@ plainto_tsquery('english', $1)
		ORDER BY rank DESC
		LIMIT $3
	`, query, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DocumentResult
	for rows.Next() {
		var doc DocumentResult
		if err := rows.Scan(&doc.ID, &doc.Title, &doc.Content, &doc.TextRank); err != nil {
			return nil, err
		}
		doc.SourceAccountID = accountID
		results = append(results, doc)
	}
	return results, rows.Err()
}

// float32SliceToPgVector converts a Go float32 slice to pgvector literal: [0.1,0.2,...].
func float32SliceToPgVector(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	sb := strings.Builder{}
	sb.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(fmt.Sprintf("%g", f))
	}
	sb.WriteByte(']')
	return sb.String()
}
