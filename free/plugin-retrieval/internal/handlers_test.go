package internal

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHealth_NoDB verifies Health returns 503 when DB is nil.
func TestHealth_NoDB(t *testing.T) {
	h := &Handlers{DB: nil}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

// TestSearch_MissingBody returns 400 on empty body.
func TestSearch_MissingBody(t *testing.T) {
	h := &Handlers{DB: nil}
	req := httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(""))
	w := httptest.NewRecorder()
	h.Search(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestIndex_MissingFields returns 400 when id or content is missing.
func TestIndex_MissingFields(t *testing.T) {
	h := &Handlers{DB: nil}
	body, _ := json.Marshal(map[string]string{"title": "Only title"})
	req := httptest.NewRequest(http.MethodPost, "/index", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Index(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestFloat32SliceToPgVector verifies vector literal format.
func TestFloat32SliceToPgVector(t *testing.T) {
	cases := []struct {
		input    []float32
		expected string
	}{
		{[]float32{0.1, 0.2, 0.3}, "[0.1,0.2,0.3]"},
		{[]float32{}, "[]"},
		{nil, "[]"},
	}
	for _, c := range cases {
		got := float32SliceToPgVector(c.input)
		if got != c.expected {
			t.Errorf("float32SliceToPgVector(%v) = %q, want %q", c.input, got, c.expected)
		}
	}
}
