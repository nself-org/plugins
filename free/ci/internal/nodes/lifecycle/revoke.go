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
	unlock, lerr := d.lockNode(id)
	if lerr != nil {
		return rep, newErr(CodeNoRoute, "lock", "cannot lock node %s: %s", id, oneLine(lerr))
	}
	defer unlock()
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
		return rep, err
	}
	if perr := d.pins().Remove(pinAlias(id)); perr != nil {
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
