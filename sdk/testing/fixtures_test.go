package testing_test

import (
	"io"
	"net/http"
	"testing"
	"time"

	sdktest "github.com/nself-org/plugin-sdk/testing"
)

func TestStubUpstream(t *testing.T) {
	srv := sdktest.StubUpstream(t, map[string]any{
		"/v1/models": []string{"model-a", "model-b"},
	})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatalf("GET /v1/models: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestStubUpstream_NotFound(t *testing.T) {
	srv := sdktest.StubUpstream(t, map[string]any{
		"/v1/exists": "yes",
	})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/missing")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestMustJSON(t *testing.T) {
	b := sdktest.MustJSON(t, map[string]string{"key": "value"})
	if len(b) == 0 {
		t.Error("MustJSON returned empty bytes")
	}
}

func TestTempCacheDir(t *testing.T) {
	dir := sdktest.TempCacheDir(t)
	if dir == "" {
		t.Error("TempCacheDir returned empty string")
	}
}

func TestWithTimeout(t *testing.T) {
	ctx := sdktest.WithTimeout(t, 5*time.Second)
	if ctx == nil {
		t.Error("WithTimeout returned nil context")
	}
}

func TestFixedClock(t *testing.T) {
	fixed := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := sdktest.FixedClock(fixed)
	t1 := clock()
	t2 := clock()
	if t1.IsZero() {
		t.Error("FixedClock() returned zero time")
	}
	if !t1.Equal(t2) {
		t.Errorf("FixedClock() not fixed: %v != %v", t1, t2)
	}
	if !t1.Equal(fixed) {
		t.Errorf("FixedClock() = %v, want %v", t1, fixed)
	}
}

func TestDoJSONRequest(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"ok":true}`)
	})
	status, body := sdktest.DoJSONRequest(t, handler, http.MethodGet, "/test", nil)
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if body == nil {
		t.Error("body is nil")
	}
}
