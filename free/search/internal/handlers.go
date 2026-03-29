package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

// --- Request types -----------------------------------------------------------

// CreateIndexRequest is the JSON body for POST /v1/indexes.
type CreateIndexRequest struct {
	Name             string                 `json:"name"`
	Description      *string                `json:"description,omitempty"`
	SourceTable      *string                `json:"source_table,omitempty"`
	SourceIDColumn   string                 `json:"source_id_column,omitempty"`
	SearchableFields []string               `json:"searchable_fields"`
	FilterableFields []string               `json:"filterable_fields,omitempty"`
	SortableFields   []string               `json:"sortable_fields,omitempty"`
	RankingRules     []string               `json:"ranking_rules,omitempty"`
	Settings         map[string]interface{} `json:"settings,omitempty"`
	Engine           string                 `json:"engine,omitempty"`
}

// DocumentRequest represents a single document to index.
type DocumentRequest struct {
	ID     string                 `json:"id"`
	Fields map[string]interface{} `json:"-"`
}

// UnmarshalJSON custom-unmarshals a document: extracts "id" and puts everything
// else into Fields.
func (d *DocumentRequest) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if id, ok := raw["id"]; ok {
		if s, ok := id.(string); ok {
			d.ID = s
		} else {
			b, _ := json.Marshal(id)
			d.ID = strings.Trim(string(b), `"`)
		}
	}
	delete(raw, "id")
	d.Fields = raw
	return nil
}

// IndexDocumentsRequest wraps a batch of documents.
type IndexDocumentsRequest struct {
	Documents []DocumentRequest `json:"documents"`
}

// SearchRequestBody is the JSON body for POST /v1/search.
type SearchRequestBody struct {
	Q       string                 `json:"q"`
	Indexes []string               `json:"indexes,omitempty"`
	Filter  map[string]interface{} `json:"filter,omitempty"`
	Sort    []string               `json:"sort,omitempty"`
	Limit   int                    `json:"limit,omitempty"`
	Offset  int                    `json:"offset,omitempty"`
}

// RegisterRoutes mounts all search endpoints on the given router.
func RegisterRoutes(r chi.Router, pool *pgxpool.Pool) {
	// Index management.
	r.Post("/v1/indexes", handleCreateIndex(pool))
	r.Get("/v1/indexes", handleListIndexes(pool))
	r.Delete("/v1/indexes/{name}", handleDeleteIndex(pool))

	// Document management.
	r.Post("/v1/indexes/{name}/documents", handleIndexDocuments(pool))
	r.Delete("/v1/indexes/{name}/documents/{id}", handleDeleteDocument(pool))

	// Search.
	r.Post("/v1/search", handleSearch(pool))

	// Suggestions / autocomplete.
	r.Get("/v1/search/suggest", handleSuggest(pool))
}

// --- Handlers ----------------------------------------------------------------

func handleCreateIndex(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateIndexRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		if req.Name == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		if len(req.SearchableFields) == 0 {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "searchable_fields is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		idx, err := CreateIndex(ctx, pool, req)
		if err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		sdk.Respond(w, http.StatusCreated, idx)
	}
}

func handleListIndexes(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		indexes, err := ListIndexes(ctx, pool)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if indexes == nil {
			indexes = []SearchIndex{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"indexes": indexes,
			"count":   len(indexes),
		})
	}
}

func handleDeleteIndex(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		if name == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		deleted, err := DeleteIndex(ctx, pool, name)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !deleted {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "index not found"})
			return
		}

		sdk.Respond(w, http.StatusOK, map[string]bool{"deleted": true})
	}
}

func handleIndexDocuments(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		if name == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "index name is required"})
			return
		}

		var req IndexDocumentsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		if len(req.Documents) == 0 {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "documents array is required"})
			return
		}
		if len(req.Documents) > 1000 {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "maximum 1000 documents per batch"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		indexed := 0
		for _, doc := range req.Documents {
			if _, err := IndexDocument(ctx, pool, name, doc); err == nil {
				indexed++
			}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"indexed": indexed,
			"total":   len(req.Documents),
			"failed":  len(req.Documents) - indexed,
		})
	}
}

func handleDeleteDocument(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		docID := chi.URLParam(r, "id")
		if name == "" || docID == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "index name and document id are required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		deleted, err := DeleteDocument(ctx, pool, name, docID)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !deleted {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "document not found"})
			return
		}

		sdk.Respond(w, http.StatusOK, map[string]bool{"deleted": true})
	}
}

func handleSearch(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req SearchRequestBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		if strings.TrimSpace(req.Q) == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "query parameter \"q\" is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		result, err := Search(ctx, pool, req)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		sdk.Respond(w, http.StatusOK, result)
	}
}

func handleSuggest(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if strings.TrimSpace(q) == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "query parameter \"q\" is required"})
			return
		}

		limit := parseIntDefault(r.URL.Query().Get("limit"), 10)

		// Parse indexes from comma-separated query parameter.
		var indexes []string
		if v := r.URL.Query().Get("indexes"); v != "" {
			for _, s := range strings.Split(v, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					indexes = append(indexes, s)
				}
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		result, err := Suggest(ctx, pool, q, indexes, limit)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		sdk.Respond(w, http.StatusOK, result)
	}
}
