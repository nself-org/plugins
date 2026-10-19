package lifecycle

// Purpose: `nodes revoke` and `nodes remove` (ADR 0025 §10): the store aggregate revokes the node, expires
// its in-flight leases and runs every registered revoker hook in one transaction; then the pin is removed
// and the live-session closers run, and a partial failure there is reported by name.
// Inputs:  a node id.
// Outputs: RevokeReport; one audit row (node.revoke or node.remove).
// Constraints: a failing revoker rolls the whole transaction back (state and leases stay) and the error is
//          returned; after the commit the node IS revoked, so a failing pin removal or session closer
//          is listed in Remaining and returned as E657 partial_revoke (the operator sees what is left);
//          nSelf never edits the node's authorized_keys (ADR 0023): the report says so.

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// RevokeReport says what a revoke or remove did and what is left.
type RevokeReport struct {
	NodeID        string   `json:"node"`
	Removed       bool     `json:"removed"`
	LeasesExpired int64    `json:"leases_expired"`
	PinRemoved    bool     `json:"pin_removed"`
	SessionsShut  int      `json:"sessions_closed"`
	Remaining     []string `json:"remaining"`
	Note          string   `json:"note"`
}

const authorizedKeysNote = "nself does not edit the node's authorized_keys; remove the CI key there yourself"

// Revoke revokes a node.
func (d *Deps) Revoke(ctx context.Context, id string) (RevokeReport, error) {
	return d.revoke(ctx, id, false)
}

// Remove revokes a node and deletes its record; the audit rows stay.
func (d *Deps) Remove(ctx context.Context, id string) (RevokeReport, error) {
	return d.revoke(ctx, id, true)
}

func (d *Deps) revoke(ctx context.Context, id string, remove bool) (RevokeReport, error) {
	rep := RevokeReport{NodeID: id, Remaining: []string{}, Note: authorizedKeysNote}
	// No node lock here: an emergency revoke must never wait behind a stuck command. The store aggregate
	// is one CAS transaction; the pin is removed under the pin lock; a rotate re-checks revocation under
	// the same pin lock before it writes.
	hooks := ext.revokerList()
	var err error
	if remove {
		r, e := d.Store.NodeRemove(ctx, id, d.Actor, hooks)
		rep.LeasesExpired, rep.Removed, err = r.LeasesExpired, r.Removed, e
	} else {
		r, e := d.Store.NodeRevoke(ctx, id, d.Actor, hooks)
		rep.LeasesExpired, err = r.LeasesExpired, e
	}
	if err != nil {
		if remove && CodeOf(err) == CodeNotFound {
			return d.dropOrphanPin(ctx, rep, id, err)
		}
		return rep, err
	}
	if perr := d.withPins(func(p remote.PinnedHostKeys) error { return p.Remove(pinAlias(id)) }); perr != nil {
		rep.Remaining = append(rep.Remaining, "pinned host key not removed")
	} else {
		rep.PinRemoved = true
	}
	for i, closer := range ext.closerList() {
		if cerr := closer(ctx, id); cerr != nil {
			rep.Remaining = append(rep.Remaining, fmt.Sprintf("session closer %d failed", i+1))
			continue
		}
		rep.SessionsShut++
	}
	if len(rep.Remaining) > 0 {
		return rep, newErr(CodeRevoked, ReasonPartial, "node %s is revoked but cleanup is incomplete: %s", id, strings.Join(rep.Remaining, "; "))
	}
	return rep, nil
}

// dropOrphanPin lets `nodes remove <id>` clear a pin left by an interrupted `nodes add` (the node was
// never registered, so there is no record to revoke). Without a pin it is the original E650.
func (d *Deps) dropOrphanPin(ctx context.Context, rep RevokeReport, id string, notFound error) (RevokeReport, error) {
	// Under the node lock an in-flight `nodes add` of this id is finished, so a pin without a record is
	// really orphaned; if the record appeared meanwhile, the pin is live and the remove runs normally.
	unlock, lerr := d.lockNode(id)
	if lerr != nil {
		return rep, newErr(CodeNoRoute, "lock", "node %s is busy: %s", id, oneLine(lerr))
	}
	defer unlock()
	if _, gerr := d.Registry.Get(ctx, id); gerr == nil {
		unlock()
		unlock = func() {}
		return d.revoke(ctx, id, true)
	}
	dropped := false
	err := d.withPins(func(p remote.PinnedHostKeys) error {
		have, perr := p.Lookup(pinAlias(id))
		if perr != nil || len(have) == 0 {
			return notFound
		}
		dropped = true
		return p.Remove(pinAlias(id))
	})
	if err != nil || !dropped {
		return rep, notFound
	}
	rep.PinRemoved = true
	rep.Note = "no node " + id + " is registered; the stale pinned host key was removed"
	return rep, nil
}
