// Package internal provides Reciprocal Rank Fusion (RRF) scoring for hybrid retrieval.
//
// Purpose: Merge ranked result lists from pgvector ANN and tsvector BM25 using RRF.
//   RRF formula: score(doc) = Σ 1/(k + rank_in_list) for each list containing the doc.
//   k=60 is the standard smoothing constant (Cormack et al. 2009).
//
// Inputs: two ranked lists (vector results, text results) of DocumentResult.
// Outputs: single merged list sorted by RRF score descending.
// Constraints:
//   - Documents appearing in only one list receive the RRF score from that list alone.
//   - Documents appearing in both lists receive additive scores (higher recall from fusion).
//   - topK controls the maximum number of results returned.
//   - source_account_id isolation is enforced at query time (never mixed in fusion).
package internal

import "sort"

// RRFConstant is the smoothing constant k per Cormack et al. 2009.
const RRFConstant = 60.0

// DocumentResult represents a single retrieved document with its metadata.
type DocumentResult struct {
	ID              string  `json:"id"`
	SourceAccountID string  `json:"source_account_id"`
	Title           string  `json:"title"`
	Content         string  `json:"content"`
	Similarity      float64 `json:"similarity,omitempty"`  // pgvector cosine similarity
	TextRank        float64 `json:"text_rank,omitempty"`   // tsvector ts_rank
	RRFScore        float64 `json:"rrf_score"`
}

// RRFMerge merges two ranked result lists using Reciprocal Rank Fusion.
//   vectorResults: ranked by pgvector cosine similarity (index 0 = most similar).
//   textResults:   ranked by tsvector ts_rank (index 0 = highest rank).
//   topK:          maximum results to return (0 = return all).
// Returns results sorted by RRF score descending.
func RRFMerge(vectorResults, textResults []DocumentResult, topK int) []DocumentResult {
	scores := make(map[string]float64)
	byID := make(map[string]DocumentResult)

	// Accumulate RRF scores from vector results
	for rank, doc := range vectorResults {
		scores[doc.ID] += 1.0 / (RRFConstant + float64(rank+1))
		byID[doc.ID] = doc
	}

	// Accumulate RRF scores from text results
	for rank, doc := range textResults {
		scores[doc.ID] += 1.0 / (RRFConstant + float64(rank+1))
		if _, exists := byID[doc.ID]; !exists {
			byID[doc.ID] = doc
		}
	}

	// Build merged result slice with RRF scores
	merged := make([]DocumentResult, 0, len(scores))
	for id, score := range scores {
		doc := byID[id]
		doc.RRFScore = score
		merged = append(merged, doc)
	}

	// Sort by RRF score descending
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].RRFScore > merged[j].RRFScore
	})

	if topK > 0 && len(merged) > topK {
		return merged[:topK]
	}
	return merged
}
