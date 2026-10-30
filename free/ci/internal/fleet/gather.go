package fleet

// Purpose: one status pass over the registry: list the registered nodes, read the recent local
// results, ask every node in parallel and merge the rows.
// Inputs:  an open store, a Reader and Options.
// Outputs: Report, rows sorted by node id.
// Constraints: read-only on the store; revoked nodes are left out; a node that does not answer
//          is a row, never an error of the pass.

import (
	"context"
	"sort"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Gather runs one status pass.
func Gather(ctx context.Context, st *store.Store, read Reader, o Options) (Report, error) {
	rep := Report{Nodes: []Row{}}
	if st == nil {
		return rep, nil
	}
	all, err := registry.New(st).List(ctx)
	if err != nil {
		return rep, err
	}
	var nodes []registry.Node
	for _, n := range all {
		if n.Record.State == "revoked" || n.Capability.Lifecycle.RevokedAt != nil {
			continue
		}
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Record.ID < nodes[j].Record.ID })
	results, err := LastResults(ctx, st, MaxResults)
	if err != nil {
		return rep, err
	}
	replies := Ask(ctx, nodes, read, o)
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	at := now()
	for i, n := range nodes {
		row := BuildRow(n, replies[i], results, at)
		if row.Source == SourceUnreachable {
			rep.Unreachable++
		}
		rep.Nodes = append(rep.Nodes, row)
	}
	return rep, nil
}

// LastResults returns up to limit runs of the local store, newest first (the rows of `ci runs list`).
func LastResults(ctx context.Context, st *store.Store, limit int) ([]Result, error) {
	out := []Result{}
	if st == nil {
		return out, nil
	}
	page, err := st.History(ctx, 0, limit)
	if err != nil {
		return nil, err
	}
	for _, p := range page.Pipelines {
		out = append(out, Result{ID: p.ID, Project: p.Project, Revision: p.Revision, Status: p.Status})
	}
	return out, nil
}
