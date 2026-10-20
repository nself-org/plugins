package main

// Purpose: the audit sealer of the V15 `ci serve` (D-1273). Rows that other packages write through the core
// audit API are not sealed by their writer; the serve chains them on a timer so `audit --verify` does not
// fail on a TEAM coordinator once a row is older than audit.SealInterval.
// Inputs:  the serve flag --audit-seal-interval (default 15s; a duration, or "off"); the coordinator's store.
// Outputs: sealed audit rows; one log line per failed pass.
// Constraints: registers through coordinator.RegisterListener like the other serve parts; the loop stops
//          with the serve and the stop function waits for the pass in flight.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func init() {
	if err := coordinator.RegisterListener("audit-sealer", "audit-seal-interval", "15s", startAuditSealer); err != nil {
		panic("nself-ci: " + err.Error())
	}
}

// startAuditSealer runs Log.RunSealer every addr (a duration) until the serve stops; "off" starts nothing.
func startAuditSealer(ctx context.Context, addr string, c *coordinator.Coordinator) (func(context.Context) error, error) {
	if ctx.Err() != nil || strings.EqualFold(strings.TrimSpace(addr), "off") {
		return noop, nil
	}
	every, err := time.ParseDuration(strings.TrimSpace(addr))
	if err != nil || every <= 0 || every > audit.SealInterval {
		return nil, fmt.Errorf("--audit-seal-interval must be \"off\" or a duration up to %s", audit.SealInterval)
	}
	log := &audit.Log{St: c.Store(), Scrub: cirun.Redactions}
	gctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		log.RunSealer(gctx, every, func(err error) { c.Logf("audit sealer: %v", err) })
	}()
	return func(sctx context.Context) error {
		cancel()
		select {
		case <-done:
		case <-sctx.Done():
		}
		return nil
	}, nil
}
