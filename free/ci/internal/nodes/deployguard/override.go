package deployguard

// Purpose: the add-time decision (P7-NODE-14, Epic D9): match the node being added, set its deploy_host
// fact on the draft capability, and handle `--allow-deploy-host` with an interactive confirmation and
// one audit row.
// Inputs:  AddRequest (node id, destination, the draft capability, the flag, a confirmer).
// Outputs: the Result; the draft's DeployHost set (matched, source, override); one audit row for a
//          confirmed override.
// Constraints: a match without the flag registers the node ineligible (deploy_host.matched true); the flag
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
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && line == "" {
			return false, nil
		}
		return strings.TrimSpace(line) == want, nil
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
}

// CheckAdd refreshes the set, matches the node and applies the override rules. The returned warnings are
// for the operator (a failed refresh or lookup); they never block.
func (g *Guard) CheckAdd(ctx context.Context, q AddRequest) (Result, []string, error) {
	var warn []string
	if err := g.Refresh(ctx); err != nil {
		warn = append(warn, err.Error())
	}
	id := Identity{Dest: q.Dest, Names: []string{q.NodeID}}
	if q.Capability != nil {
		c := IdentityOf(*q.Capability)
		id.Names = append(id.Names, c.Names...)
		id.Fingerprints, id.MachineID = c.Fingerprints, c.MachineID
		if id.Dest == "" {
			id.Dest = c.Dest
		}
	}
	r, w, err := g.Match(ctx, id)
	warn = append(warn, w...)
	if err != nil || !r.Matched {
		return r, warn, err
	}
	dh := DeployHostFor(r, model.CapabilityDeployHost{})
	if q.Allow {
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
