package backup

// Purpose: what a restore reads from the data dir it replaces and writes back into the restored state:
//          the running-coordinator check, revocations and the deny list (merged, never dropped), the team
//          mode, and the empty-dir test (Epic D21, review F15).
// Inputs:  the store of the data dir being replaced (before the swap) and the restored store (after it).
// Outputs: a prior snapshot; merge counts.
// Constraints: a certificate or node revoked in the replaced dir stays refused after a restore of an older
//          backup; the deny list only grows; a team data dir never goes back to personal; all writes use
//          the store's own audited calls.

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// liveWindow is how fresh a coordinator heartbeat must be to count as running (3 x the store TTL).
const liveWindow = 90 * time.Second

// prior is the snapshot of the data dir being replaced.
type prior struct {
	existed bool // state.db existed before this restore
	empty   bool // no nodes and no PKI authority: nothing to merge, nothing to protect
	team    bool // the coordinator mode row said team
	denied  map[string][]string
	revoked []string
}

func (o Options) alive(pid int) bool {
	if o.Alive != nil {
		return o.Alive(pid)
	}
	return processAlive(pid)
}

// running names a coordinator whose heartbeat is fresh and whose process exists.
func running(ctx context.Context, o Options, st *store.Store) (string, error) {
	cs, err := st.Coordinators(ctx)
	if err != nil {
		return "", fail(ReasonIO, "the coordinator table cannot be read: %v", err)
	}
	now := o.now()
	for _, c := range cs {
		if now.Sub(time.Unix(0, c.HeartbeatAt)) <= liveWindow && o.alive(c.PID) {
			return c.ID, nil
		}
	}
	return "", nil
}

// snapshot reads the deny list, the revoked nodes, the mode and the emptiness of the dir being replaced.
func snapshot(ctx context.Context, st *store.Store, existed bool) (prior, error) {
	p := prior{existed: existed, denied: map[string][]string{}}
	nodes, err := st.NodeList(ctx)
	if err != nil {
		return p, fail(ReasonIO, "the node registry cannot be read: %v", err)
	}
	auth, err := st.PKIAuthorities(ctx)
	if err != nil {
		return p, fail(ReasonIO, "the PKI cannot be read: %v", err)
	}
	p.empty = !existed || (len(nodes) == 0 && len(auth) == 0)
	for _, n := range nodes {
		if n.State == "revoked" {
			p.revoked = append(p.revoked, n.ID)
		}
	}
	rows, err := st.PKIDenied(ctx, 0)
	if err != nil {
		return p, fail(ReasonIO, "the deny list cannot be read: %v", err)
	}
	for _, r := range rows {
		p.denied[r.NodeID] = append(p.denied[r.NodeID], r.Serial)
	}
	mode, found, err := st.CoordinatorModeGet(ctx)
	p.team = err != nil || (found && mode == store.CoordinatorTeam) // an unreadable mode reads team (fail closed)
	return p, nil
}

// mergeCounts says what mergePrior carried over.
type mergeCounts struct{ denied, revoked int64 }

// mergePrior writes the replaced dir's deny list and revocations into the restored store.
func mergePrior(ctx context.Context, st *store.Store, p prior, actor string) (mergeCounts, error) {
	var c mergeCounts
	ids := make([]string, 0, len(p.denied))
	for id := range p.denied {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		n, err := st.PKIDeny(ctx, actor, id, "restore_merge", p.denied[id]...)
		if err != nil {
			return c, fail(ReasonIO, "the deny list cannot be merged: %v", err)
		}
		c.denied += n
	}
	for _, id := range p.revoked {
		rec, err := st.NodeGet(ctx, id)
		var se *store.Error
		if errors.As(err, &se) && se.Code == "E650" {
			continue // the node is not in the backup; its certificates are denied above
		}
		if err != nil {
			return c, fail(ReasonIO, "node %s cannot be read: %v", id, err)
		}
		if rec.State == "revoked" {
			continue
		}
		if _, err = st.NodeRevoke(ctx, id, actor, nil); err != nil {
			return c, fail(ReasonIO, "node %s cannot be re-revoked: %v", id, err)
		}
		c.revoked++
	}
	if p.team {
		if _, err := st.CoordinatorModeEnsure(ctx, true, actor); err != nil {
			return c, fail(ReasonIO, "the team mode cannot be kept: %v", err)
		}
	}
	return c, nil
}
