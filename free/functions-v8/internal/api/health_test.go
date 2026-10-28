package api

import (
	"encoding/json"
	"github.com/nself-org/nself-functions-v8/internal/isolate"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthMigrationFailureJSON(t *testing.T) {
	for _, failed := range []bool{false, true} {
		h := &Handler{pool: isolate.NewPool("", 1, 0, 0, nil), Migrations: func(w http.ResponseWriter) map[string]int {
			if failed {
				w.WriteHeader(http.StatusServiceUnavailable)
				return map[string]int{"applied": -1, "expected": -1}
			}
			return map[string]int{"applied": 2, "expected": 2}
		}}
		w := httptest.NewRecorder()
		h.Health(w, httptest.NewRequest(http.MethodGet, "/health", nil))
		res := w.Result()
		want := http.StatusOK
		if failed {
			want = http.StatusServiceUnavailable
		}
		if res.StatusCode != want || res.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("failed=%v: status=%d content-type=%q", failed, res.StatusCode, res.Header.Get("Content-Type"))
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
