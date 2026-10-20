package main

// Purpose: what serveWire gives a running `ci serve` (P7-NODE-20): a live row in the coordinator table, so
//          `serve restore` refuses on an idle coordinator, and the incarnation the dispatcher reads.

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/backup"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func TestServeRegistersSoRestoreRefusesAnIdleCoordinator(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	dir := serveFixture(t, quietHost())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listening := make(chan struct{})
	hook := func(c *coordinator.Config) {
		c.Listen = func(network, _ string) (net.Listener, error) {
			defer close(listening)
			return net.Listen(network, "127.0.0.1:0")
		}
	}
	exit := make(chan int, 1)
	go func() {
		exit <- serveV15(ctx, []string{"--insecure", "--workdir", filepath.Join(dir, "wd"), "--data-dir", dir, "--agent-addr", "", "--enroll-addr", ""}, hook)
	}()
	select {
	case <-listening:
	case code := <-exit:
		t.Fatalf("serve exited %d before listening", code)
	case <-time.After(15 * time.Second):
		t.Fatal("serve never listened")
	}
	st, err := cirun.OpenStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	cs, err := st.Coordinators(context.Background())
	if err != nil || len(cs) == 0 {
		t.Fatalf("coordinators = %v, %v: serve did not register itself", cs, err)
	}
	if n, err := st.HighestIncarnation(context.Background()); err != nil || n < 1 {
		t.Fatalf("highest incarnation = %d, %v", n, err)
	}
	kek, err := os.ReadFile(pki.KEKPath(dir))
	if err != nil {
		t.Fatalf("no KEK in the data dir: %v", err)
	}
	var buf bytes.Buffer
	if _, err = backup.Create(context.Background(), backup.Options{DataDir: dir}, &buf); err != nil {
		t.Fatal(err)
	}
	_, err = backup.Restore(context.Background(), backup.Options{DataDir: dir}, &buf, kek)
	if got := backup.ReasonOf(err); got != backup.ReasonRunningCoordinator {
		t.Fatalf("restore over a running idle serve = %v (reason %q), want running_coordinator", err, got)
	}
	cancel()
	if code := <-exit; code != 0 {
		t.Fatalf("serve exit %d", code)
	}
}

func TestServeRefusesWhileARestoreIsPending(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	dir := serveFixture(t, quietHost())
	if err := os.WriteFile(filepath.Join(dir, "restore.pending"), []byte(`{"empty":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if code := serveV15(ctx, []string{"--insecure", "--workdir", filepath.Join(dir, "wd"), "--data-dir", dir}, nil); code != 1 {
		t.Fatalf("serve over an unfinished restore exited %d, want 1", code)
	}
}
