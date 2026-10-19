package main

// Purpose: the production boundary of `ci serve` (D-1218, Epic D9): refuse to start on a deploy host, and
// keep deploy_host fresh while the daemon runs (refresh at start and every deployguard.RefreshEvery).
// Inputs:  this host (serveLocal), the CI home (serveHome) and its state.db.
// Outputs: nil from startServeGuard, or the E663 error that makes serve exit 1; refresh failures go to
//          stderr as warnings and never stop the daemon.
// Constraints: the logic lives in internal/nodes/deployguard; this file only wires it to serve. The seams
//          are variables so tests never read the developer's host or CI home.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/serve"
)

// stopWait bounds how long serve waits for the refresh loop to close the state database on exit.
const stopWait = 5 * time.Second

var (
	serveLocal = func() deployguard.Local { return deployguard.NewLocal(deployguard.FunnelRun) }
	serveHome  = lifecycle.CIHome
	serveRun   = serve.RunServe
	// serveRefresh runs Guard.Run against the node state database until ctx ends.
	serveRefresh = func(ctx context.Context, home string, onErr func(error)) {
		if err := os.MkdirAll(home, 0o700); err != nil {
			onErr(err)
			return
		}
		st, err := deployguard.OpenStore(filepath.Join(home, "state.db"))
		if err != nil {
			onErr(err)
			return
		}
		defer func() { _ = st.Close() }()
		newGuard(st, home, adminActor()).Run(ctx, onErr)
	}
)

// startServeGuard checks this host, then starts the refresh loop. stop ends the loop and waits briefly
// for it; it is nil when the start is refused.
func startServeGuard(ctx context.Context) (stop func(), err error) {
	home := serveHome()
	warns, err := deployguard.StartGuard(ctx, home, serveLocal(), "nself-ci serve")
	for _, w := range warns {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	if err != nil {
		return nil, err
	}
	rctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		serveRefresh(rctx, home, func(e error) { fmt.Fprintln(os.Stderr, "warning: deploy-host refresh:", e) })
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(stopWait):
		}
	}, nil
}
