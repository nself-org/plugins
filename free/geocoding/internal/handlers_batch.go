package internal

import (
	"fmt"
	"net/http"
	"sync"
)

// ─── Batch forward — POST /geocode/batch-forward ─────────────────────────────

func (h *Handlers) BatchForward(w http.ResponseWriter, r *http.Request) {
	if !h.checkRateLimit(w, r) {
		return
	}
	var body struct {
		Queries []string `json:"queries"`
	}
	if err := readJSON(r, &body); err != nil || len(body.Queries) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": "queries array is required"})
		return
	}
	maxBatch := h.Cfg.MaxBatchSize
	if maxBatch <= 0 {
		maxBatch = 100
	}
	if len(body.Queries) > maxBatch {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "bad_request",
			"message": fmt.Sprintf("batch size %d exceeds maximum %d", len(body.Queries), maxBatch),
		})
		return
	}

	if h.Prov == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "provider_error",
			"message": "no geocoding provider configured",
		})
		return
	}

	account := sa(r)
	type batchItem struct {
		Query   string           `json:"query"`
		Results []map[string]any `json:"results"`
		Error   string           `json:"error,omitempty"`
	}

	items := make([]batchItem, len(body.Queries))
	var wg sync.WaitGroup
	for i, q := range body.Queries {
		wg.Add(1)
		go func(idx int, query string) {
			defer wg.Done()
			results, err := h.Prov.Forward(r.Context(), query)
			item := batchItem{Query: query}
			if err != nil {
				item.Error = err.Error()
			} else {
				out := make([]map[string]any, 0, len(results))
				for _, res := range results {
					out = append(out, providerResultToMap(res, false))
				}
				if len(results) > 0 {
					h.cacheResult(r.Context(), account, "forward", query, results[0])
				}
				item.Results = out
			}
			items[idx] = item
		}(i, q)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "count": len(items)})
}

// ─── Batch reverse — POST /geocode/batch-reverse ─────────────────────────────

func (h *Handlers) BatchReverse(w http.ResponseWriter, r *http.Request) {
	if !h.checkRateLimit(w, r) {
		return
	}
	var body struct {
		Points []struct {
			Lat float64 `json:"lat"`
			Lng float64 `json:"lng"`
		} `json:"points"`
	}
	if err := readJSON(r, &body); err != nil || len(body.Points) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": "points array is required"})
		return
	}
	maxBatch := h.Cfg.MaxBatchSize
	if maxBatch <= 0 {
		maxBatch = 100
	}
	if len(body.Points) > maxBatch {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "bad_request",
			"message": fmt.Sprintf("batch size %d exceeds maximum %d", len(body.Points), maxBatch),
		})
		return
	}

	if h.Prov == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "provider_error",
			"message": "no geocoding provider configured",
		})
		return
	}

	account := sa(r)
	type batchItem struct {
		Lat    float64        `json:"lat"`
		Lng    float64        `json:"lng"`
		Result map[string]any `json:"result,omitempty"`
		Error  string         `json:"error,omitempty"`
	}

	items := make([]batchItem, len(body.Points))
	var wg sync.WaitGroup
	for i, pt := range body.Points {
		wg.Add(1)
		go func(idx int, lat, lng float64) {
			defer wg.Done()
			item := batchItem{Lat: lat, Lng: lng}
			res, err := h.Prov.Reverse(r.Context(), lat, lng)
			if err != nil {
				item.Error = err.Error()
			} else {
				queryText := fmt.Sprintf("%.7f,%.7f", lat, lng)
				h.cacheResult(r.Context(), account, "reverse", queryText, res)
				item.Result = providerResultToMap(res, false)
			}
			items[idx] = item
		}(i, pt.Lat, pt.Lng)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "count": len(items)})
}
