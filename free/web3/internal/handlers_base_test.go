// Purpose: Tests for Health/Ready/sa/writeJSON/marshalJSON helper functions.
// Inputs: httptest requests and recorders, mocked pgx pool for Ready.
// Outputs: asserts status codes, JSON bodies, header defaults.
// Constraints: No real Postgres.
package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	h := &Handlers{}
	rec := httptest.NewRecorder()
	h.Health(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["status"] != "ok" || out["plugin"] != "web3" {
		t.Errorf("body = %v", out)
	}
}

func TestReady_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectPing()
	rec := httptest.NewRecorder()
	h.Ready(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestReady_DBDown(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectPing().WillReturnError(context.DeadlineExceeded)
	rec := httptest.NewRecorder()
	h.Ready(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestSA_DefaultPrimary(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	if got := h.sa(r); got != "primary" {
		t.Errorf("sa = %q; want primary", got)
	}
}

func TestSA_HeaderOverride(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("X-Source-Account-ID", "acct-1")
	if got := h.sa(r); got != "acct-1" {
		t.Errorf("sa = %q; want acct-1", got)
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]int{"a": 1})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Error("content-type missing")
	}
}

func TestMarshalJSON_Nil(t *testing.T) {
	if got := marshalJSON(nil); got != nil {
		t.Errorf("marshalJSON(nil) = %v; want nil", got)
	}
}

func TestMarshalJSON_Value(t *testing.T) {
	got := marshalJSON(map[string]string{"a": "b"})
	if got == nil {
		t.Fatal("marshalJSON should return non-nil bytes")
	}
	var out map[string]string
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatal(err)
	}
	if out["a"] != "b" {
		t.Errorf("out = %v", out)
	}
}
