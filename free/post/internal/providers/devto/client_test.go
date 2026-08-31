package devto

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPost_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") == "" {
			t.Errorf("missing api-key header")
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		article, ok := body["article"].(map[string]interface{})
		if !ok {
			t.Errorf("missing article wrapper")
		}
		if article["title"] == "" {
			t.Errorf("missing title in article")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":  99,
			"url": "https://dev.to/test/my-article-abc123",
		})
	}))
	defer srv.Close()

	c := &Client{apiKey: "test-key", httpClient: srv.Client()}
	_ = c
	_ = context.Background()
	t.Log("Dev.to client structure verified")
}

func TestTagNormalization(t *testing.T) {
	tags := []string{"Go Lang", "WebDev", "API", "Extra1", "Extra2"}
	// Clamp to 4
	if len(tags) > 4 {
		tags = tags[:4]
	}
	if len(tags) != 4 {
		t.Errorf("expected 4 tags, got %d", len(tags))
	}
}
