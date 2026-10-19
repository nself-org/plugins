package deployguard

// Purpose: the add-time decision (P7-NODE-14, Epic D9): match the node being added, set its deploy_host
// fact on the draft capability, and handle `--allow-deploy-host` with an interactive confirmation and
// one audit row.
// Inputs:  AddRequest (node id, destination, the draft capability, the flag, a confirmer).
// Outputs: the Result; the draft's DeployHost set (matched, source, override); one audit row for a
//          confirmed override.
// Constraints: a match on the destination alone is refused before the host is contacted (CheckPreAdd); a match
//          found only after contact (fingerprint, machine id) registers the node ineligible (deploy_host.matched true); the flag
//          without a match changes nothing; the flag with a match and no confirmation (no terminal, or
//          anything but the node id typed back) fails with E663 and registers nothing; the audit row is
//          written before the node exists and names the source and kind, never a key.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Confirmer asks the operator to type want back; it returns true only on an exact answer.
type Confirmer func(prompt, want string) (bool, error)

// TerminalConfirm reads the answer from in. tty false means no confirmation is possible.
func TerminalConfirm(in io.Reader, out io.Writer, tty bool) Confirmer {
	return func(prompt, want string) (bool, error) {
		if !tty {
			return false, newErr(ReasonNoTTY, "--allow-deploy-host needs an interactive terminal to confirm")
		}
		_, _ = fmt.Fprintf(out, "%s\nType the node id (%s) to continue: ", prompt, want)
		line, _ := bufio.NewReader(in).ReadString('\n')
		return want != "" && strings.TrimSpace(line) == want, nil
	}
}

// AddRequest is one node being added or paired.
type AddRequest struct {
	NodeID, Dest string
	// Capability is the draft; CheckAdd sets its DeployHost. Nil when only the answer is wanted.
	Capability *model.Capability
	Allow      bool
	Confirm    Confirmer
	Actor      string
	// Confirmed says an earlier CheckPreAdd for this node already confirmed and audited the override: CheckAdd
	// then neither asks nor writes a second audit row.
	Confirmed bool
}

// CheckPreAdd is the check before the host is contacted (`nodes add`): the destination and node id alone, with
// identity from ssh -G and DNS. A match without Allow is refused here, so nothing is scanned, probed or installed
// on a deploy host; with Allow the override is confirmed and audited now and the caller passes Confirmed to the
// later CheckAdd. An unmatched destination is left to CheckAdd, which also sees the host key and machine id.
func (g *Guard) CheckPreAdd(ctx context.Context, q AddRequest) (Result, []string, error) {
	q.Capability, q.Confirmed = nil, false
	r, warn, err := g.CheckAdd(ctx, q)
	if err == nil && r.Matched && !q.Allow {
		err = newErr(ReasonMatched, "%s is a deploy host (%s); it was not contacted and not added (--allow-deploy-host adds it after a confirmation)", q.NodeID, r.Source)
	}
	return r, warn, err
}

// CheckAdd refreshes the set, matches the node and applies the override rules. The returned warnings are
// for the operator (a failed refresh or lookup); an unmatched node is refused only when a source has never
// been read at all (unverified).
func (g *Guard) CheckAdd(ctx context.Context, q AddRequest) (Result, []string, error) {
	var warn []string
	if err := g.Refresh(ctx); err != nil {
		warn = append(warn, err.Error())
	}
	id := Identity{Dest: q.Dest, Names: []string{q.NodeID}}
	if q.Capability != nil {
		c := IdentityOf(*q.Capability)
		id.Names = append(id.Names, c.Names...)
		id.Hostname, id.Fingerprints, id.MachineID = c.Hostname, c.Fingerprints, c.MachineID
		if id.Dest == "" {
			id.Dest = c.Dest
		}
	}
	r, w, err := g.Match(ctx, id)
	warn = append(warn, w...)
	if err != nil {
		return r, warn, err
	}
	if !r.Matched {
		if g.nothingKnown(ctx) {
			warn = append(warn, "no deploy inventory is known and the never-list is empty: nothing was checked against deploy hosts (run inside a project with .nself or set ci.project_roots)")
		}
		if r.SSHFailed {
			// With `ssh -G` down the real host behind an alias is unknown. A DNS-only gap is a warning: a node
			// behind ProxyJump may have a name only the jump host resolves, and name matches need no DNS.
			return r, warn, newErr(ReasonNoIdentity, "the identity of %s could not be resolved (%s); node was not added", q.NodeID, strings.Join(w, "; "))
		}
		return r, warn, g.unverified(ctx)
	}
	dh := DeployHostFor(r, model.CapabilityDeployHost{})
	if q.Allow && q.Confirmed {
		dh.Override = true
	} else if q.Allow {
		if q.Confirm == nil {
			return r, warn, newErr(ReasonNoTTY, "--allow-deploy-host needs an interactive terminal to confirm")
		}
		msg := fmt.Sprintf("Node %s is a deploy host (%s, %s). CI jobs would run untrusted code beside production data.", q.NodeID, r.Source, r.Kind)
		ok, cerr := q.Confirm(msg, q.NodeID)
		if cerr != nil {
			return r, warn, cerr
		}
		if !ok {
			return r, warn, newErr(ReasonNotConfirmed, "override of %s was not confirmed; the node was not added", q.NodeID)
		}
		actor := q.Actor
		if actor == "" {
			actor = g.actor()
		}
		if err = g.Store.AppendAudit(ctx, actor, "node.deploy_host.override", q.NodeID, fmt.Sprintf("confirmed source=%s kind=%s", r.Source, r.Kind)); err != nil {
			return r, warn, err
		}
		dh.Override = true
	}
	if q.Capability != nil {
		q.Capability.DeployHost = dh
	}
	return r, warn, nil
}

// unverified refuses an unmatched add while some source has never been read successfully: with no cached
// entries for it, "no match" proves nothing. Fix the source (nself on PATH, the config file, the project
// root) and retry; a source that worked once keeps its cached entries and does not block.
func (g *Guard) unverified(ctx context.Context) error {
	states, err := g.Store.ExclusionStates(ctx)
	if err != nil {
		return err
	}
	for _, st := range states {
		if !st.OK && st.LastOKAt == 0 {
			return newErr(ReasonInventory, "deploy hosts cannot be verified: %s has never been read (%s); node was not added", st.Root, st.Error)
		}
	}
	return nil
}

// nothingKnown is true when no project root has been read and the never-list holds nothing: an unmatched
// result then means "unchecked", and the operator is told so.
func (g *Guard) nothingKnown(ctx context.Context) bool {
	s, err := g.Set(ctx)
	if err != nil || !s.Empty() {
		return false
	}
	roots, err := g.Store.ExclusionProjects(ctx)
	return err == nil && len(roots) == 0
}
