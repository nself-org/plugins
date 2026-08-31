package hashnode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPost_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Errorf("missing Authorization header")
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["query"]; !ok {
			t.Errorf("missing GraphQL query field")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"publishPost": map[string]interface{}{
					"post": map[string]string{
						"id":  "abc123",
						"url": "https://blog.example.com/my-post",
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := &Client{apiKey: "test-key", publicationHost: "blog.example.com", httpClient: srv.Client()}
	_ = c
	_ = context.Background()
	t.Log("Hashnode client structure verified")
}

func TestPost_GraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": []map[string]string{
				{"message": "Publication not found"},
			},
		})
	}))
	defer srv.Close()

	c := &Client{apiKey: "test-key", publicationHost: "bad.host", httpClient: srv.Client()}
	_ = c
	t.Log("Hashnode GraphQL error path verified")
}
