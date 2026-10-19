package lifecycle

// Purpose: the human output of the nodes commands: small aligned tables and one-line results.
// Inputs:  NodeView, AddResult, TestResult, RevokeReport.
// Outputs: text on the writer.
// Constraints: only ids, states, trust fields and public fingerprints are printed.

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

func renderLine(w io.Writer, s string) { _, _ = fmt.Fprintln(w, s) }

func join(v []string) string {
	if len(v) == 0 {
		return "-"
	}
	return strings.Join(v, ",")
}

func renderList(w io.Writer, nodes []NodeView) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NODE\tSTATE\tPLATFORM\tPROJECTS\tACCEPTS\tAGENT")
	for _, n := range nodes {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", n.ID, n.State, orDash(n.Platform), join(n.AuthorizedProjects), join(n.Trust.Accepts), orDash(n.AgentVersion))
	}
	_ = tw.Flush()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func boolText(b *bool) string {
	switch {
	case b == nil:
		return "unknown"
	case *b:
		return "yes"
	}
	return "no"
}

func renderShow(w io.Writer, n NodeView) {
	rows := [][2]string{
		{"node", n.ID}, {"name", n.Name}, {"state", n.State}, {"platform", orDash(n.Platform)}, {"transport", n.Transport},
		{"discovered", fmt.Sprint(n.Discovered)}, {"authorized projects", join(n.AuthorizedProjects)}, {"eligible", fmt.Sprint(n.Eligible)},
		{"revoked", fmt.Sprint(n.RevokedAt != nil)}, {"accepts", join(n.Trust.Accepts)}, {"secret classes", join(n.Trust.SecretClasses)},
		{"uid separation", boolText(n.Separation.UIDSeparation)}, {"signing eligible", boolText(n.Separation.SigningEligible)},
		{"agent", orDash(n.AgentVersion)}, {"host keys", join(n.HostKeyFingerprints)},
	}
	for _, r := range rows {
		_, _ = fmt.Fprintf(w, "%-20s %s\n", r[0], r[1])
	}
}

func renderAdd(w io.Writer, n NodeView, r AddResult) {
	_, _ = fmt.Fprintf(w, "node %s added: %s, agent %s, separation %s\n", n.ID, orDash(n.Platform), r.AgentVersion, r.Separation.Reason)
	_, _ = fmt.Fprintf(w, "authorized for project %s; accepts %s; pinned %s\n", join(n.AuthorizedProjects), join(n.Trust.Accepts), join(r.Fingerprints))
}

func renderTest(w io.Writer, r TestResult) {
	_, _ = fmt.Fprintf(w, "node %s: connected, agent %s, %d changed fact(s)\n", r.NodeID, r.AgentVersion, len(r.Drift))
	for _, p := range r.Drift {
		_, _ = fmt.Fprintf(w, "  changed %s\n", p)
	}
}

func renderRevoke(w io.Writer, r RevokeReport) {
	verb := "revoked"
	if r.Removed {
		verb = "removed"
	}
	_, _ = fmt.Fprintf(w, "node %s %s: %d lease(s) expired, pin removed %v, %d session(s) closed\n", r.NodeID, verb, r.LeasesExpired, r.PinRemoved, r.SessionsShut)
	for _, rem := range r.Remaining {
		_, _ = fmt.Fprintf(w, "  left: %s\n", rem)
	}
	_, _ = fmt.Fprintln(w, r.Note)
}
