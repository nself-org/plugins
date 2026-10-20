package main

// Purpose: tests of the audit sealer inside a running V15 `ci serve` (D-1273): a row written through the
// core audit API without sealing is chained by the serve itself, and --audit-seal-interval off leaves it.
// Inputs:  serveV15 with a short seal interval, a short-path data dir, a free TCP port.
// Outputs: pass or fail.
// Constraints: 127.0.0.1 only; the data dir is under the system temp dir (sun_path is 104 bytes).

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordclient"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// sealedRow reports whether the audit row with this target carries a chain head.
func sealedRow(t *testing.T, st *store.Store, target string) (found, sealed bool) {
	t.Helper()
	es, err := st.AuditEntries(context.Background(), 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		if e.Target == target {
			return true, len(e.Head) > 0
		}
	}
	return false, false
}

func TestServeSealsRowsOtherPackagesWrite(t *testing.T) {
	for _, c := range []struct {
		name, interval string
		wantSealed     bool
	}{{"default-on with a short interval", "50ms", true}, {"off", "off", false}} {
		t.Run(c.name, func(t *testing.T) {
			serveFixture(t, quietHost())
			data, err := os.MkdirTemp("", "ci-seal")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(data) })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			hook := func(cfg *coordinator.Config) {
				cfg.Listen = func(network, _ string) (net.Listener, error) { return net.Listen(network, "127.0.0.1:0") }
			}
			args := []string{"--insecure", "--workdir", filepath.Join(data, "wd"), "--data-dir", data, "--api-addr", freePort(t), "--audit-seal-interval", c.interval}
			done := make(chan int, 1)
			go func() { done <- serveV15(ctx, args, hook) }()
			waitUntil(t, "the serve lock", func() bool { return coordclient.ServeHoldsLock(data) })
			st, err := store.Open(filepath.Join(data, "state.db"), store.Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = st.Close() }()
			target := "d1273/" + c.name
			if err = st.AuditAppend(ctx, store.AuditInput{Actor: "test", Action: "test.write", Target: target}); err != nil {
				t.Fatal(err)
			}
			if c.wantSealed {
				waitUntil(t, "the serve to seal the row", func() bool { _, s := sealedRow(t, st, target); return s })
			} else {
				time.Sleep(300 * time.Millisecond)
				if found, sealed := sealedRow(t, st, target); !found || sealed {
					t.Fatalf("found=%v sealed=%v, want an unsealed row", found, sealed)
				}
			}
			cancel()
			if code := <-done; code != 0 {
				t.Fatalf("serve exit = %d", code)
			}
		})
	}
}
