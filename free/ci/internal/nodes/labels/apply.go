package labels

// Purpose: the label edit of `nself ci nodes label` (P7-NODE-30): add and remove labels of one node, or
// read them, with the audit rows written by the store in the same transaction.
// Inputs:  Env (store, registry, actor), a node id or the alias `local`, labels to set and keys to remove.
// Outputs: the node's label set after the edit, sorted by key.
// Constraints: a node must be registered (the alias `local` is the invoking machine's runner and needs no
//          record); a rejected edit changes nothing; labels are assigned here only, never taken from a
//          hello or a probe; the `nself.` prefix cannot be set by this path.

import (
	"context"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// LocalNode is the alias of the invoking machine's runner.
const LocalNode = "local"

// Env is what the label and project operations need.
type Env struct {
	Store    *store.Store
	Registry *registry.Registry
	Actor    string
	// Team is true on a TEAM coordinator: no wildcard grants, no directory arguments, no `local`.
	Team bool
	// ResolveDir returns the repository id of a directory (the way evidence names it); nil refuses directories.
	ResolveDir func(ctx context.Context, dir string) (string, bool)
}

// Labels returns the node's assigned labels.
func (e Env) Labels(ctx context.Context, node string) ([]store.NodeLabel, error) {
	if err := e.checkNode(ctx, node); err != nil {
		return nil, err
	}
	return e.Store.NodeLabels(ctx, node)
}

// Edit sets the labels in add (a `key` or `key=value` each, replacing a label of the same key), removes
// the keys in remove and returns the resulting set.
func (e Env) Edit(ctx context.Context, node string, add, remove []string) ([]store.NodeLabel, error) {
	if err := e.checkNode(ctx, node); err != nil {
		return nil, err
	}
	set, err := Parse(add)
	if err != nil {
		return nil, err
	}
	have, err := e.Store.NodeLabels(ctx, node)
	if err != nil {
		return nil, err
	}
	drop := map[string]bool{}
	for _, k := range remove {
		drop[k] = true
	}
	for _, l := range set {
		drop[l.Key] = true
	}
	next := set
	for _, l := range have {
		if !drop[l.Key] {
			next = append(next, l)
		}
	}
	sort.Slice(next, func(i, j int) bool { return next[i].Key < next[j].Key })
	if err = Validate(next); err != nil {
		return nil, err
	}
	if err = e.Store.NodeLabelsReplace(ctx, node, next, e.Actor); err != nil {
		return nil, err
	}
	return next, nil
}

func (e Env) checkNode(ctx context.Context, node string) error {
	if node == LocalNode {
		if e.Team {
			return newErr(ReasonProjectUnknown, "the alias local names the invoking machine; name a registered node on a coordinator")
		}
		return nil
	}
	_, err := e.Registry.Get(ctx, node)
	return err
}
