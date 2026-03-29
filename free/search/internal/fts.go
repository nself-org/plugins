package internal

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SearchHit is a single result from a full-text search query.
type SearchHit struct {
	ID        string `json:"id"`
	Index     string `json:"index"`
	Score     float64 `json:"score"`
	Content   string `json:"content"`
	Headline  string `json:"headline,omitempty"`
}

// SearchResponse is the envelope returned by the search endpoint.
type SearchResponse struct {
	Hits            []SearchHit `json:"hits"`
	Total           int         `json:"total"`
	Limit           int         `json:"limit"`
	Offset          int         `json:"offset"`
	Query           string      `json:"query"`
	ProcessingTimeMs int64      `json:"processingTimeMs"`
}

// Suggestion is a single autocomplete result.
type Suggestion struct {
	Value string `json:"value"`
	Index string `json:"index"`
}

// SuggestResponse is the envelope returned by the suggest endpoint.
type SuggestResponse struct {
	Suggestions      []Suggestion `json:"suggestions"`
	Query            string       `json:"query"`
	ProcessingTimeMs int64        `json:"processingTimeMs"`
}

// Search performs a PostgreSQL full-text search using to_tsvector/websearch_to_tsquery
// with ts_rank_cd for relevance scoring.
func Search(ctx context.Context, pool *pgxpool.Pool, req SearchRequestBody) (*SearchResponse, error) {
	start := time.Now()
	sourceAccountID := resolveSourceAccountID(ctx)

	limit := 20
	if req.Limit > 0 {
		limit = req.Limit
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := req.Offset

	// Determine which indexes to search.
	indexNames := req.Indexes
	if len(indexNames) == 0 {
		indexes, err := ListIndexes(ctx, pool)
		if err != nil {
			return nil, err
		}
		for _, idx := range indexes {
			indexNames = append(indexNames, idx.Name)
		}
	}

	if len(indexNames) == 0 {
		return &SearchResponse{
			Hits:             []SearchHit{},
			Total:            0,
			Limit:            limit,
			Offset:           offset,
			Query:            req.Q,
			ProcessingTimeMs: time.Since(start).Milliseconds(),
		}, nil
	}

	// Build parameterized query.
	args := []interface{}{sourceAccountID, indexNames}
	argIdx := 3

	filterSQL := ""
	if req.Filter != nil {
		for key, val := range req.Filter {
			// Safely parameterize the JSON field access.
			// Key is used in the operator expression but values are parameterized.
			filterSQL += fmt.Sprintf(" AND content->>$%d = $%d", argIdx, argIdx+1)
			args = append(args, key, fmt.Sprintf("%v", val))
			argIdx += 2
		}
	}

	tsQueryParam := fmt.Sprintf("$%d", argIdx)
	args = append(args, strings.TrimSpace(req.Q))
	queryArgIdx := argIdx
	argIdx++

	limitParam := fmt.Sprintf("$%d", argIdx)
	args = append(args, limit)
	argIdx++

	offsetParam := fmt.Sprintf("$%d", argIdx)
	args = append(args, offset)

	// Main search query with ts_rank_cd scoring.
	searchSQL := fmt.Sprintf(`
		SELECT
			source_id,
			index_name,
			content::text,
			ts_rank_cd(search_vector, websearch_to_tsquery('english', %s)) AS score
		FROM np_search_documents
		WHERE source_account_id = $1
			AND index_name = ANY($2)
			AND search_vector @@ websearch_to_tsquery('english', %s)
			%s
		ORDER BY score DESC
		LIMIT %s OFFSET %s`,
		tsQueryParam, tsQueryParam, filterSQL, limitParam, offsetParam)

	rows, err := pool.Query(ctx, searchSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}
	defer rows.Close()

	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.ID, &h.Index, &h.Content, &h.Score); err != nil {
			return nil, err
		}
		// Round score to 6 decimal places for cleanliness.
		h.Score = math.Round(h.Score*1e6) / 1e6
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if hits == nil {
		hits = []SearchHit{}
	}

	// Count total matching documents (without limit/offset).
	countArgs := args[:len(args)-2] // strip limit and offset
	countSQL := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM np_search_documents
		WHERE source_account_id = $1
			AND index_name = ANY($2)
			AND search_vector @@ websearch_to_tsquery('english', $%d)
			%s`, queryArgIdx, filterSQL)

	var total int
	if err := pool.QueryRow(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count query failed: %w", err)
	}

	return &SearchResponse{
		Hits:             hits,
		Total:            total,
		Limit:            limit,
		Offset:           offset,
		Query:            req.Q,
		ProcessingTimeMs: time.Since(start).Milliseconds(),
	}, nil
}

// Suggest returns autocomplete suggestions using PostgreSQL trigram similarity
// (pg_trgm). It extracts individual words from indexed documents and ranks them
// by prefix match + similarity score.
func Suggest(ctx context.Context, pool *pgxpool.Pool, q string, indexes []string, limit int) (*SuggestResponse, error) {
	start := time.Now()
	sourceAccountID := resolveSourceAccountID(ctx)

	if limit <= 0 {
		limit = 10
	}

	indexNames := indexes
	if len(indexNames) == 0 {
		idxs, err := ListIndexes(ctx, pool)
		if err != nil {
			return nil, err
		}
		for _, idx := range idxs {
			indexNames = append(indexNames, idx.Name)
		}
	}

	if len(indexNames) == 0 {
		return &SuggestResponse{
			Suggestions:      []Suggestion{},
			Query:            q,
			ProcessingTimeMs: time.Since(start).Milliseconds(),
		}, nil
	}

	sql := `
		SELECT DISTINCT word, index_name, similarity(word, $3) AS score
		FROM (
			SELECT
				unnest(string_to_array(regexp_replace(content::text, '[^a-zA-Z0-9 ]', ' ', 'g'), ' ')) AS word,
				index_name
			FROM np_search_documents
			WHERE source_account_id = $1 AND index_name = ANY($2)
		) words
		WHERE word ILIKE $3 || '%' AND length(word) > 2
		ORDER BY score DESC, word
		LIMIT $4`

	rows, err := pool.Query(ctx, sql, sourceAccountID, indexNames, q, limit)
	if err != nil {
		return nil, fmt.Errorf("suggest query failed: %w", err)
	}
	defer rows.Close()

	var suggestions []Suggestion
	for rows.Next() {
		var s Suggestion
		var score float64
		if err := rows.Scan(&s.Value, &s.Index, &score); err != nil {
			return nil, err
		}
		suggestions = append(suggestions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if suggestions == nil {
		suggestions = []Suggestion{}
	}

	return &SuggestResponse{
		Suggestions:      suggestions,
		Query:            q,
		ProcessingTimeMs: time.Since(start).Milliseconds(),
	}, nil
}

// parseIntDefault parses a string as int, returning defaultVal on failure.
func parseIntDefault(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
