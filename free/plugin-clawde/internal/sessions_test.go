package internal

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHealth_NoDB returns 503 when DB is nil.
func TestHealth_NoDB(t *testing.T) {
	h := &Handlers{DB: nil}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

// TestCreateSession_MissingID returns 400 when id is absent.
func TestCreateSession_MissingID(t *testing.T) {
	h := &Handlers{DB: nil}
	body, _ := json.Marshal(map[string]string{"source_account_id": "acc1"})
	req := httptest.NewRequest(http.MethodPost, "/sessions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.CreateSession(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestCreateSession_InvalidJSON returns 400 on malformed body.
func TestCreateSession_InvalidJSON(t *testing.T) {
	h := &Handlers{DB: nil}
	req := httptest.NewRequest(http.MethodPost, "/sessions", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	h.CreateSession(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestAppendEvent_MissingEventType returns 400 when event_type absent.
func TestAppendEvent_MissingEventType(t *testing.T) {
	h := &Handlers{DB: nil}
	body, _ := json.Marshal(map[string]string{"payload": "data"})
	req := httptest.NewRequest(http.MethodPost, "/sessions/sess-1/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.AppendEvent(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestAppendEvent_InvalidJSON returns 400 on malformed body.
func TestAppendEvent_InvalidJSON(t *testing.T) {
	h := &Handlers{DB: nil}
	req := httptest.NewRequest(http.MethodPost, "/sessions/sess-1/events", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	h.AppendEvent(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
