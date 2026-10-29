package fleet

// Purpose: the human table of `ci nodes status`: one line per node, then the reasons of every node
// that is not ready or did not answer.
// Inputs:  Report. Outputs: text on w.

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Render writes the status table.
func Render(w io.Writer, rep Report) error {
	if len(rep.Nodes) == 0 {
		_, err := fmt.Fprintln(w, "no nodes registered")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NODE\tSTATE\tVERSION\tPOOL\tREADY\tWATCHER\tLAST PASS\tQUEUED\tRUNNING\tLAST RESULTS")
	for _, r := range rep.Nodes {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Node, dash(r.State), dash(r.AgentVersion),
			dash(strings.Join(r.Pools, ",")), readyText(r), r.Watcher, dash(r.LastPass), count(r.Queued), count(r.Running), results(r.LastResults))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, r := range rep.Nodes {
		if r.Ready != nil && *r.Ready || len(r.ReadyWhy) == 0 {
			continue
		}
		if _, err := fmt.Fprintf(w, "%s: %s\n", r.Node, strings.Join(r.ReadyWhy, "; ")); err != nil {
			return err
		}
	}
	return nil
}

func readyText(r Row) string {
	switch {
	case r.Source == SourceUnreachable:
		return "unreachable"
	case r.Ready == nil:
		return "unknown"
	case *r.Ready:
		return "ready"
	}
	return "not ready"
}

func count(v *int) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprint(*v)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func results(rs []Result) string {
	if len(rs) == 0 {
		return "-"
	}
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = r.Status
	}
	return strings.Join(parts, ",")
}
