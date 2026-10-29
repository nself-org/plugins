package clientapi

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestStoredEventReplayKeepsBytesAndSequences(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	records := []string{
		`{"schema_version":"1","data":{"type":"queued","schema":"ci.events/v1","seq":1,"run_id":"r"}}`,
		`{"schema_version":"1","data":{"type":"started","schema":"ci.events/v1","seq":2,"run_id":"r", "job_id":"build"}}`,
		`{"schema_version":"1","data":{"type":"result","schema":"ci.events/v1","seq":3,"run_id":"r","evidence":{"original":true}}}`,
	}
	for i, raw := range records {
		typ := "started"
		if i == 2 {
			typ = "result"
		}
		if err := st.RecordEvent(context.Background(), "r", int64(i+1), typ, []byte(raw)); err != nil {
			t.Fatal(err)
		}
	}
	s := New(Config{Backend: &StoreBackend{Store: st}})
	out := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	if !s.journalEvents(context.Background(), out, req, item("r", "acme/app", "passed"), 2) {
		t.Fatal("journal was not used")
	}
	if got, want := out.Body.String(), records[1]+"\n"+records[2]+"\n"; got != want {
		t.Fatalf("replay changed bytes: %s", got)
	}
}
