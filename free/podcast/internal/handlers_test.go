package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	h := &Handlers{}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["plugin"] != "podcast" {
		t.Fatalf("expected plugin=podcast, got %s", body["plugin"])
	}
}

func TestSourceAccountDefault(t *testing.T) {
	h := &Handlers{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if sa := h.sa(req); sa != "primary" {
		t.Fatalf("expected primary, got %s", sa)
	}
}

func TestCreatePodcastValidation(t *testing.T) {
	h := &Handlers{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/podcasts", nil)
	w := httptest.NewRecorder()
	h.CreatePodcast(w, req)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestXmlEscape(t *testing.T) {
	input := `<script>alert("xss")&</script>`
	expected := `&lt;script&gt;alert(&quot;xss&quot;)&amp;&lt;/script&gt;`
	if got := xmlEscape(input); got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}
