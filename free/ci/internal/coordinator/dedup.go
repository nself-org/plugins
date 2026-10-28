package coordinator

// Purpose: the semantic key, the maintenance loop (retention, claim heartbeat, recovery) of the V15
// coordinator (P7-NODE-10, Epic D7, X7).
// Inputs:  project, revision, trigger class and policy digest of a trigger; the injected clock.
// Outputs: a stable semantic key; a store that forgets what is older than the 72 h redelivery horizon.
// Constraints: the key hashes length-prefixed fields, so no field boundary can be forged; dedup lookups
//          and the enqueue itself live in internal/store (one transaction).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// semanticKey is the key that coalesces triggers for the same code under the same policy. A trigger that
// names a pipeline keys on it too, so a manual "release" run never folds into a live default run; the key
// of a trigger with no pipeline is the plain four-field one.
func semanticKey(project, revision, class, policy, pipeline string) string {
	h := sha256.New()
	fields := []string{project, revision, class, policy}
	if pipeline != "" {
		fields = append(fields, "pipeline="+pipeline)
	}
	for _, f := range fields {
		h.Write([]byte(strconv.Itoa(len(f)) + ":" + f))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// maintain recovers rows a dead coordinator left claimed, refreshes the claims of this one and prunes
// what is older than the retention. It runs once at start and then every Beat.
func (c *Coordinator) maintain(ctx context.Context) {
	var pruned time.Time
	for {
		st := c.cfg.Store
		if _, err := st.IntakeRecover(ctx, c.cfg.ID, c.cfg.StaleAfter); err != nil && ctx.Err() == nil {
			c.cfg.Logf("intake recovery failed: %v", err)
		}
		if err := st.IntakeBeat(ctx, c.cfg.ID); err != nil && ctx.Err() == nil {
			c.cfg.Logf("intake heartbeat failed: %v", err)
		}
		if now := c.cfg.Clock.Now(); now.Sub(pruned) >= time.Hour {
			c.prune(ctx, now)
			pruned = now
		}
		c.nudge()
		select {
		case <-c.cfg.Clock.After(c.cfg.Beat):
		case <-ctx.Done():
			return
		}
	}
}

// MarkRetention is how long the marks of one provider_mark name stay. KeyPrefix narrows the rule to the
// keys that start with it ("" is every key of the name); a mark older than MaxAge is deleted.
type MarkRetention struct {
	Name      string
	KeyPrefix string
	MaxAge    time.Duration
}

// DefaultMarkRetention is the horizon of the GitHub reconcile marks (keys are "<kind>|<repo>|<id>"): a run
// update and a demand job are read again only while GitHub still lists them, so a week is generous; the
// head (term, enq) and redelivery marks keep 30 days, far beyond the 72 h redelivery horizon.
var DefaultMarkRetention = []MarkRetention{
	{Name: "github-reconcile", KeyPrefix: "run|", MaxAge: 7 * 24 * time.Hour},
	{Name: "github-reconcile", KeyPrefix: "demand|", MaxAge: 7 * 24 * time.Hour},
	{Name: "github-reconcile", MaxAge: 30 * 24 * time.Hour},
}

// prune forgets what is past its retention: finished intake rows and keys, and the provider marks.
func (c *Coordinator) prune(ctx context.Context, now time.Time) {
	st := c.cfg.Store
	if _, err := st.DedupPrune(ctx, 0); err != nil && ctx.Err() == nil {
		c.cfg.Logf("intake prune failed: %v", err)
	}
	rules := c.cfg.MarkRetention
	if rules == nil {
		rules = DefaultMarkRetention
	}
	for _, r := range rules {
		if _, err := st.ProviderMarkPrune(ctx, r.Name, r.KeyPrefix, now.Add(-r.MaxAge)); err != nil && ctx.Err() == nil {
			c.cfg.Logf("provider mark prune failed (%s): %v", r.Name, err)
		}
	}
}

// dispatchLoop hands queued rows to the Dispatcher, at most Concurrency at a time, until ctx ends.
func (c *Coordinator) dispatchLoop(ctx context.Context) {
	if c.cfg.Dispatch == nil {
		return
	}
	sem := make(chan struct{}, c.cfg.Concurrency)
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return
		}
		var row store.IntakeRow
		ok, err := false, error(nil)
		if !c.paused.Load() {
			row, ok, err = c.cfg.Store.IntakeClaim(ctx, c.cfg.ID)
		}
		if err != nil || !ok {
			<-sem
			if err != nil && ctx.Err() == nil {
				c.cfg.Logf("intake claim failed: %v", err)
			}
			select {
			case <-c.wake:
			case <-time.After(c.cfg.Poll):
			case <-ctx.Done():
				return
			}
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			c.runRow(ctx, row)
			c.nudge()
		}()
	}
}

// runRow dispatches one claimed row and records how it ended. A dispatch cut short by shutdown goes back
// to the queue, so the pipeline still runs after the next start.
func (c *Coordinator) runRow(ctx context.Context, row store.IntakeRow) {
	res, err := c.cfg.Dispatch(ctx, row)
	end := context.WithoutCancel(ctx)
	switch {
	case err == nil: // a finished pipeline is recorded even when shutdown began meanwhile: it must not run twice
		err = c.cfg.Store.IntakeFinish(end, row.ID, c.cfg.ID, res.PipelineID, "exit="+strconv.Itoa(res.Exit), true)
	case ctx.Err() != nil:
		err = c.cfg.Store.IntakeRequeue(end, row.ID, c.cfg.ID)
	default:
		err = c.cfg.Store.IntakeFinish(end, row.ID, c.cfg.ID, res.PipelineID, "error: "+truncate(err.Error(), 200), false)
	}
	if err != nil {
		c.cfg.Logf("intake row %s not recorded: %v", row.ID, err)
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
