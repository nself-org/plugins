package main

// Purpose: `nself ci runs outbox [--json]` (P7-HOST-02, Epic D7): the commit
// statuses this machine still owes a code host. It lists pending entries, entries
// parked as failed (with the attempt count, the last error and the time of the
// next attempt) and suppressed entries (with the reason: the project's
// reporting owner is the team coordinator).
// Inputs:  argv after `outbox`; env NSELF_CI_HOME.
// Outputs: a table or one v1 JSON envelope; exit 0, 1 (usage) or 2 (infrastructure).
// Constraints: V15 only (registered through the runs registry); read only; no
// store on this machine is an empty list.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	// The blank import registers the providers.report_owner policy key.
	_ "github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const outboxUsage = "nself ci runs outbox [--json]"

func init() { registerRunsSub("outbox", runOutbox) }

// outboxView is one row of the output.
type outboxView struct {
	Repo          string `json:"repo"`
	Revision      string `json:"revision"`
	Seq           int64  `json:"seq"`
	State         string `json:"state"`
	Attempts      int    `json:"attempts"`
	NextAttemptAt string `json:"next_attempt_at,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

func runOutbox(args []string) int {
	var asJSON bool
	for _, a := range args {
		if a != "--json" && a != "-json" && a != "--json=true" {
			return usageErr(outboxUsage)
		}
		asJSON = true
	}
	list := []outboxView{}
	st, err := openHistory(false)
	if err != nil {
		return exitForError(err)
	}
	if st != nil {
		defer func() { _ = st.Close() }()
		if list, err = outboxRows(context.Background(), st); err != nil {
			return exitForError(err)
		}
	}
	if asJSON {
		if err = output.WriteData(os.Stdout, "ci runs outbox", list, nil); err != nil {
			return exitForError(err)
		}
		return 0
	}
	if len(list) == 0 {
		fmt.Println("no commit status is waiting to be posted")
		return 0
	}
	for _, v := range list {
		fmt.Printf("%-10s %-30s %-7s seq %d, attempts %d%s\n", v.State, v.Repo, shortSHA(v.Revision), v.Seq, v.Attempts, outboxNote(v))
	}
	return 0
}

// outboxRows lists the entries an operator needs to see.
func outboxRows(ctx context.Context, st *store.Store) ([]outboxView, error) {
	items, err := st.OutboxList(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]outboxView, 0, len(items))
	for _, it := range items {
		v := outboxView{Repo: strings.TrimPrefix(it.Context, "nself-ci@"), Revision: it.Revision, Seq: it.Seq, State: it.State, Attempts: it.Attempts, LastError: it.LastError, Reason: it.Reason}
		if it.State == store.OutboxFailed && it.NextAttemptAt > 0 {
			v.NextAttemptAt = time.Unix(0, it.NextAttemptAt).UTC().Format(time.RFC3339)
		}
		out = append(out, v)
	}
	return out, nil
}

func shortSHA(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}

func outboxNote(v outboxView) string {
	switch {
	case v.Reason != "":
		return ": " + v.Reason
	case v.NextAttemptAt != "":
		return fmt.Sprintf(": next attempt %s (%s)", v.NextAttemptAt, v.LastError)
	}
	return ""
}
