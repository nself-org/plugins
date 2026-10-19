package audit

// Purpose: tests of the chained audit log: chain and verify, tamper detection, unsealed rows from other
// writers, the secret scrubber, AppendTx, and concurrent appenders in one process and a second one.
// Inputs:  a real SQLite store in t.TempDir() with an injected clock.
// Outputs: pass or fail; no network.
// Constraints: the second process is this test binary re-run with an env switch.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/store"
	_ "modernc.org/sqlite"
)

var epoch = time.Unix(1_800_000_000, 0)

type clock struct{ n atomic.Int64 }

func (c *clock) now() time.Time { return epoch.Add(time.Duration(c.n.Add(1)) * time.Millisecond) }
func (c *clock) advance(d time.Duration) {
	c.n.Add(int64(d / time.Millisecond))
}

func newLog(t *testing.T) (*Log, *clock, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	clk := &clock{}
	st, err := store.Open(path, store.Options{Clock: clk.now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return &Log{St: st}, clk, path
}

func ev(i int) Event {
	return Event{Actor: "operator:test", Action: "test.event", Target: fmt.Sprintf("t%d", i), Detail: "n=" + fmt.Sprint(i)}
}

func exec1(t *testing.T, path, q string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

func TestChainAndVerify(t *testing.T) {
	l, _, _ := newLog(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := l.Append(ctx, ev(i)); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := l.Verify(ctx, 0)
	if err != nil || !rep.OK() || rep.Rows != 5 || rep.Sealed != 5 || rep.Unsealed != 0 {
		t.Fatalf("verify = %+v, %v", rep, err)
	}
	rows, _ := l.St.AuditEntries(ctx, 0, 0)
	if len(rows) != 5 || len(rows[0].Head) != 32 || rows[0].PrevHead != nil || string(rows[1].PrevHead) != string(rows[0].Head) {
		t.Fatalf("chain shape wrong: %+v", rows[:2])
	}
}

func TestVerifyDetectsEditedRow(t *testing.T) {
	l, _, path := newLog(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		_ = l.Append(ctx, ev(i))
	}
	exec1(t, path, "UPDATE audit SET detail='forged' WHERE seq=2")
	rep, err := l.Verify(ctx, 0)
	if err != nil || rep.OK() || rep.BadSeq != 2 {
		t.Fatalf("an edited row must be named: %+v, %v", rep, err)
	}
}

func TestVerifyDetectsDeletedRow(t *testing.T) {
	l, _, path := newLog(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		_ = l.Append(ctx, ev(i))
	}
	exec1(t, path, "DELETE FROM audit WHERE seq=2")
	rep, _ := l.Verify(ctx, 0)
	if rep.OK() || rep.BadSeq != 3 {
		t.Fatalf("a deleted middle row must break the link at the next row: %+v", rep)
	}
}

func TestUnsealedRowsFromOtherWriters(t *testing.T) {
	l, clk, _ := newLog(t)
	ctx := context.Background()
	_ = l.Append(ctx, ev(0))
	// A NODE mutation writes through the CI-31 API, which does not seal.
	if err := l.St.AppendAudit(ctx, "operator:node", "node.add", "n1", "added"); err != nil {
		t.Fatal(err)
	}
	rep, _ := l.Verify(ctx, 0)
	if !rep.OK() || rep.Unsealed != 1 || rep.Overdue != 0 {
		t.Fatalf("a fresh unsealed row is reported but is not a failure: %+v", rep)
	}
	clk.advance(2 * time.Minute)
	rep, _ = l.Verify(ctx, 0)
	if rep.OK() || rep.Unsealed != 1 || rep.Overdue != 1 {
		t.Fatalf("an unsealed row past the interval fails verification: %+v", rep)
	}
	if rows, _ := l.St.AuditEntries(ctx, 0, 0); rows[1].Head != nil {
		t.Fatal("Verify must never seal")
	}
	n, err := l.Seal(ctx)
	if err != nil || n != 1 {
		t.Fatalf("seal = %d, %v", n, err)
	}
	if rep, _ = l.Verify(ctx, 0); !rep.OK() || rep.Unsealed != 0 {
		t.Fatalf("after seal: %+v", rep)
	}
	// The next Append also chains rows other packages wrote, in seq order.
	_ = l.St.AppendAudit(ctx, "operator:node", "node.add", "n2", "added")
	_ = l.St.AppendAudit(ctx, "operator:node", "node.add", "n3", "added")
	_ = l.Append(ctx, ev(9))
	if rep, _ = l.Verify(ctx, 0); !rep.OK() || rep.Sealed != 5 || rep.Unsealed != 0 {
		t.Fatalf("Append must seal pending rows first: %+v", rep)
	}
}

func TestScrubberRefusesRegisteredSecret(t *testing.T) {
	l, _, _ := newLog(t)
	r := redact.New()
	secret := []byte("s3cr3t-value-0123456789abcdef")
	if err := r.Register(secret); err != nil {
		t.Fatal(err)
	}
	l.Scrub = r
	ctx := context.Background()
	for _, e := range []Event{
		{Actor: "operator:x", Action: "a", Target: "t", Detail: "token=" + string(secret)},
		{Actor: "operator:x", Action: "a", Target: string(secret), Detail: "d"},
	} {
		err := l.Append(ctx, e)
		var ae *Error
		if !errors.As(err, &ae) || ae.Reason != ReasonSecret || ae.ErrCode() != "E687" {
			t.Fatalf("want E687 audit_secret, got %v", err)
		}
		if ae.Error() != "" && containsBytes(ae.Error(), secret) {
			t.Fatal("the error must not echo the value")
		}
	}
	if rows, _ := l.St.AuditEntries(ctx, 0, 0); len(rows) != 0 {
		t.Fatalf("a refused event writes nothing, got %d rows", len(rows))
	}
	if err := l.Append(ctx, ev(1)); err != nil {
		t.Fatalf("a clean event is accepted: %v", err)
	}
}

func containsBytes(s string, b []byte) bool {
	return len(b) > 0 && len(s) >= len(b) && (func() bool {
		for i := 0; i+len(b) <= len(s); i++ {
			if s[i:i+len(b)] == string(b) {
				return true
			}
		}
		return false
	})()
}

func TestAppendTxJoinsTheCallersTransaction(t *testing.T) {
	l, _, _ := newLog(t)
	ctx := context.Background()
	w := store.PolicyWrite{Op: store.PolicySet, Scope: "team", Subject: "p1", Key: "trust.mode", Value: []byte(`"team"`), Actor: "operator:x"}
	if err := l.St.ApplyWrite(ctx, w, func(tx *store.Tx) error {
		return AppendTx(tx, Event{Actor: "operator:x", Action: "policy.set", Target: "p1", Detail: "key=trust.mode"})
	}); err != nil {
		t.Fatal(err)
	}
	rep, _ := l.Verify(ctx, 0)
	if rep.Rows != 1 || rep.Sealed != 1 || !rep.OK() {
		t.Fatalf("the policy write's row is sealed in its own transaction: %+v", rep)
	}
	err := l.St.ApplyWrite(ctx, w, func(tx *store.Tx) error {
		if err := AppendTx(tx, Event{Actor: "operator:x", Action: "policy.set", Target: "p1", Detail: "second"}); err != nil {
			return err
		}
		return errors.New("roll back")
	})
	if err == nil {
		t.Fatal("expected the rollback error")
	}
	if rep, _ = l.Verify(ctx, 0); rep.Rows != 1 {
		t.Fatalf("a rolled-back write leaves no audit row: %+v", rep)
	}
}

func TestAppendersLoseNoRow(t *testing.T) {
	l, _, path := newLog(t)
	ctx := context.Background()
	dur := 10 * time.Second
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "AUDIT_HELPER=1", "AUDIT_DB="+path)
	out := &lockedBuf{}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var wrote atomic.Int64
	var wg sync.WaitGroup
	deadline := time.Now().Add(dur)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; time.Now().Before(deadline); i++ {
				e := Event{Actor: "operator:g", Action: "load", Target: fmt.Sprintf("g%d", g), Detail: fmt.Sprintf("i=%d", i)}
				if err := l.Append(ctx, e); err != nil {
					t.Errorf("append: %v", err)
					return
				}
				wrote.Add(1)
			}
		}(g)
	}
	wg.Wait()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("second process failed: %v\n%s", err, out.String())
	}
	var other int64
	if _, err := fmt.Sscanf(out.String(), "wrote=%d", &other); err != nil {
		t.Fatalf("second process printed no count: %q", out.String())
	}
	rep, err := l.Verify(ctx, 0)
	if err != nil || !rep.OK() || int64(rep.Rows) != wrote.Load()+other {
		t.Fatalf("rows=%d want %d (in-process %d + second process %d): %+v, %v", rep.Rows, wrote.Load()+other, wrote.Load(), other, rep, err)
	}
	if rep.Unsealed != 0 {
		t.Fatalf("every row must be sealed: %+v", rep)
	}
}

type lockedBuf struct {
	mu sync.Mutex
	b  []byte
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.b = append(l.b, p...)
	return len(p), nil
}
func (l *lockedBuf) String() string { l.mu.Lock(); defer l.mu.Unlock(); return string(l.b) }

// TestMain turns the test binary into the second appender process when AUDIT_HELPER is set.
func TestMain(m *testing.M) {
	if os.Getenv("AUDIT_HELPER") == "1" {
		os.Exit(helperAppender(os.Getenv("AUDIT_DB")))
	}
	os.Exit(m.Run())
}

func helperAppender(path string) int {
	// Open itself can meet the other process mid-write (CI-31 owns its busy handling), so retry it.
	var st *store.Store
	var err error
	for try := 0; try < 50; try++ {
		if st, err = store.Open(path, store.Options{}); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = st.Close() }()
	l := &Log{St: st}
	var n int
	for end := time.Now().Add(9 * time.Second); time.Now().Before(end); n++ {
		if err := l.Append(context.Background(), Event{Actor: "operator:p2", Action: "load", Target: "p2", Detail: fmt.Sprintf("i=%d", n)}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	fmt.Printf("wrote=%d\n", n)
	return 0
}
