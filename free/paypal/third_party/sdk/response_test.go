package sdk

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIError_Error(t *testing.T) {
	e := &APIError{Code: "not_found", Message: "resource not found"}
	if got := e.Error(); got != "resource not found" {
		t.Errorf("APIError.Error() = %q, want %q", got, "resource not found")
	}
}

func TestRespondTyped_Success(t *testing.T) {
	w := httptest.NewRecorder()
	type Payload struct {
		Name string `json:"name"`
	}
	RespondTyped(w, http.StatusOK, Payload{Name: "test"})
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp Response[Payload]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Data.Name != "test" {
		t.Errorf("data.name = %q, want %q", resp.Data.Name, "test")
	}
}

func TestRespondTypedMeta(t *testing.T) {
	w := httptest.NewRecorder()
	RespondTypedMeta(w, http.StatusOK, "hello", map[string]string{"cursor": "abc"})
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp Response[string]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Meta["cursor"] != "abc" {
		t.Errorf("meta cursor = %q, want %q", resp.Meta["cursor"], "abc")
	}
}

func TestRespond_NilBody(t *testing.T) {
	w := httptest.NewRecorder()
	Respond(w, http.StatusNoContent, nil)
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if w.Body.Len() != 0 {
		t.Errorf("body should be empty for nil body, got %q", w.Body.String())
	}
}

func TestRespondRaw(t *testing.T) {
	w := httptest.NewRecorder()
	RespondRaw(w, http.StatusCreated, map[string]string{"id": "123"})
	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}
}

func TestError_StringMessage(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, http.StatusBadRequest, "invalid input")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty error body")
	}
}

func TestError_ErrorMessage(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, http.StatusInternalServerError, errors.New("something broke"))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestError_OtherMessage(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, http.StatusUnprocessableEntity, 42) // non-string, non-error
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}
