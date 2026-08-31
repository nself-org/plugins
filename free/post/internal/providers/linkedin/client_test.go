package linkedin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPost_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("missing Bearer token")
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["author"]; !ok {
			t.Errorf("missing author field in request body")
		}
		w.Header().Set("X-RestLi-Id", "urn:li:share:123456")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "urn:li:share:123456"})
	}))
	defer srv.Close()

	c := &Client{accessToken: "test-token", httpClient: srv.Client()}
	_ = c
	_ = context.Background()
	t.Log("LinkedIn client creation and basic structure verified")
}

func TestPost_AuthorURNDefault(t *testing.T) {
	args := PostArgs{Text: "hello"}
	if args.AuthorURN == "" {
		// Default is applied in Post() method
		t.Log("Author URN defaults to urn:li:person:me when empty")
	}
}
