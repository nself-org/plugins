package detach

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func rec(typ string, seq int) string {
	return fmt.Sprintf(`{"schema_version":"1","command":"ci","data":{"type":%q,"schema":"ci.events/v1","seq":%d,"run_id":"r1"}}`, typ, seq)
}

func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	for _, l := range lines {
		if _, err = f.WriteString(l + "\n"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFollowReplayFromSeq(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r1.ndjson")
	writeLines(t, path, rec("queued", 1), "stderr noise", rec("planned", 2), rec("started", 3), rec("result", 4))
	var out bytes.Buffer
	res, err := Follow(context.Background(), &out, FollowParams{Path: path, From: 3, Poll: time.Millisecond})
	if err != nil || res.End != EndResult || res.LastSeq != 4 {
		t.Fatalf("res %+v err %v", res, err)
	}
	got := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(got) != 2 || !strings.Contains(got[0], `"seq":3`) || !strings.Contains(got[1], `"type":"result"`) || strings.Contains(out.String(), "noise") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestFollowTailsAWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r1.ndjson")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i, l := range []string{rec("queued", 1), rec("started", 2), rec("check_result", 3), rec("result", 4)} {
			time.Sleep(20 * time.Millisecond)
			writeLines(t, path, l)
			_ = i
		}
	}()
	var out bytes.Buffer
	res, err := Follow(context.Background(), &out, FollowParams{Path: path, Poll: 2 * time.Millisecond})
	wg.Wait()
	if err != nil || res.End != EndResult || strings.Count(out.String(), "\n") != 4 {
		t.Fatalf("res %+v err %v\n%s", res, err, out.String())
	}
}

func TestFollowPartialLineWaits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r1.ndjson")
	half := rec("result", 2)
	if err := os.WriteFile(path, []byte(rec("queued", 1)+"\n"+half[:20]), 0o600); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(60 * time.Millisecond)
		f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
		_, _ = f.WriteString(half[20:] + "\n")
		_ = f.Close()
	}()
	var out bytes.Buffer
	res, err := Follow(context.Background(), &out, FollowParams{Path: path, Poll: 2 * time.Millisecond})
	if err != nil || res.End != EndResult || !strings.HasSuffix(strings.TrimSpace(out.String()), half) {
		t.Fatalf("res %+v err %v\n%s", res, err, out.String())
	}
}

func TestFollowErrorEnvelopeEndsStream(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r1.ndjson")
	writeLines(t, path, rec("queued", 1), `{"schema_version":"1","command":"ci","error":{"code":"E648","message":"m","exit_code":2,"class":"infra"}}`)
	var out bytes.Buffer
	res, err := Follow(context.Background(), &out, FollowParams{Path: path, From: 9, Poll: time.Millisecond})
	if err != nil || res.End != EndError || res.Exit != 2 || !strings.Contains(out.String(), `"code":"E648"`) {
		t.Fatalf("res %+v err %v\n%s", res, err, out.String())
	}
}

func TestFollowGoneWithoutResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r1.ndjson")
	writeLines(t, path, rec("queued", 1), rec("started", 2))
	asked := 0
	var out bytes.Buffer
	res, err := Follow(context.Background(), &out, FollowParams{Path: path, Poll: time.Millisecond, Idle: 20 * time.Millisecond,
		Over: func(time.Duration) (bool, error) { asked++; return true, nil }})
	if err != nil || res.End != EndGone || res.LastSeq != 2 || asked != 1 {
		t.Fatalf("res %+v err %v asked %d", res, err, asked)
	}
}

func TestFollowStopsOnContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	_, err := Follow(ctx, &bytes.Buffer{}, FollowParams{Path: filepath.Join(t.TempDir(), "none"), Poll: time.Millisecond})
	if err == nil {
		t.Fatal("a follow of a silent file must end with the context")
	}
}

// Start renames the token-named file to the run id once the run is queued: a
// follow that opened the old name keeps reading it, through both of its passes.
func TestFollowSurvivesARename(t *testing.T) {
	dir := t.TempDir()
	token, final := filepath.Join(dir, "token.ndjson"), filepath.Join(dir, "r1.ndjson")
	writeLines(t, token, rec("queued", 1))
	rd := &Reader{Path: token}
	defer func() { _ = rd.Close() }()
	go func() {
		time.Sleep(40 * time.Millisecond)
		_ = os.Rename(token, final)
		time.Sleep(40 * time.Millisecond)
		writeLines(t, final, rec("started", 2), rec("result", 3))
	}()
	var out bytes.Buffer
	res, err := Follow(context.Background(), &out, FollowParams{Path: token, Reader: rd, Poll: 2 * time.Millisecond})
	if err != nil || res.End != EndResult || res.LastSeq != 3 || strings.Count(out.String(), "\n") != 3 {
		t.Fatalf("res %+v err %v\n%s", res, err, out.String())
	}
}
