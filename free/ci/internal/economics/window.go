package economics

// Purpose: the UTC windows budgets are counted in (Epic D11). A day budget
// counts one UTC date, a month budget (and a provider budget) one UTC month.
// Inputs:  the dispatcher's wall clock.
// Outputs: the day key (YYYY-MM-DD) and the month key (YYYY-MM) of a reservation.
// Constraints: the day key is never earlier than the largest key the store has
// recorded (a clock stepped back keeps charging the later window, review F5),
// and the month is taken from that day key, so a day and its month never disagree.

import (
	"time"

	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func dayOf(t time.Time) string { return t.UTC().Format("2006-01-02") }

// monthOf is the month key of a day key.
func monthOf(day string) string { return day[:7] }

// windows returns the day and month keys to charge now, recording the day key.
func windows(t store.LedgerTx, now time.Time) (day, month string, err error) {
	day, err = t.Window(dayOf(now))
	if err != nil {
		return "", "", err
	}
	return day, monthOf(day), nil
}

// peekWindows is windows without a write, for the world contributor and reports.
func peekWindows(high string, now time.Time) (day, month string) {
	day = max(dayOf(now), high)
	return day, monthOf(day)
}

func addSat(a, b int64) (int64, bool) { return sched.Add(a, b) }
