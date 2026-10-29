package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestRunsFollowReplaysStoredJournal(t *testing.T) {
	home := runsHome(t)
	seedRun(t, home, "r-journal", "passed", time.Now(), map[string]string{"build": "passed"})
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var want []string
	for i, typ := range []string{"queued", "started", "check_result", "result"} {
		raw := evLine(typ, "r-journal", i+1)
		if err := st.RecordEvent(context.Background(), "r-journal", int64(i+1), typ, []byte(raw)); err != nil {
			t.Fatal(err)
		}
		if i >= 1 {
			want = append(want, raw)
		}
	}
	code, out, _ := capture(t, func() int { return runFollow([]string{"r-journal", "--from-seq", "2"}) })
	if code != 0 || out != strings.Join(want, "\n")+"\n" {
		t.Fatalf("exit %d replay %s", code, out)
	}
}
