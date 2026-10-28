package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type healthWriter struct {
	*httptest.ResponseRecorder
	headers int
}

func (w *healthWriter) WriteHeader(status int) { w.headers++; w.ResponseRecorder.WriteHeader(status) }

func TestHealthMigrationFailureJSON(t *testing.T) {
	old := MigrationsHealth
	t.Cleanup(func() { MigrationsHealth = old })
	for _, failed := range []bool{false, true} {
		MigrationsHealth = func(w http.ResponseWriter) map[string]int {
			if failed {
				w.WriteHeader(http.StatusServiceUnavailable)
				return map[string]int{"applied": -1, "expected": -1}
			}
			return map[string]int{"applied": 2, "expected": 2}
		}
		w := &healthWriter{ResponseRecorder: httptest.NewRecorder()}
		HealthHandler(w, httptest.NewRequest(http.MethodGet, "/health", nil))
		res := w.Result()
		want := http.StatusOK
		if failed {
			want = http.StatusServiceUnavailable
		}
		if res.StatusCode != want || res.Header.Get("Content-Type") != "application/json" || w.headers > 1 {
			t.Fatalf("failed=%v: status=%d content-type=%q header writes=%d", failed, res.StatusCode, res.Header.Get("Content-Type"), w.headers)
		}
		var body struct {
			Migrations map[string]int `json:"migrations"`
		}
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if failed && body.Migrations["applied"] != -1 {
			t.Fatalf("missing failure counts: %v", body)
		}
	}
}
