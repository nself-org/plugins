package selftest

// Purpose: `doctor --selftest --alert`: send one notifier message per failing set per day.
// Inputs:  the selftest checks, the sinks, a state file, a clock.
// Outputs: Result; the state file holds the sets already sent today.
// Constraints: a failing set is the sorted ids of the failing selftest checks; the same
// set is sent once per UTC day, a different set (one more check failing) is a new
// message; a set no sink took is not recorded, so the next run tries again; a clean
// report sends nothing. The event carries ids and names only, never a path or log text.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/notify"
)

// Alerter sends the alert.
type Alerter struct {
	Sinks     []notify.Notifier
	StatePath string
	Host      string
	Now       func() time.Time // nil: time.Now
}

// Result says what an alert did.
type Result struct {
	Set     string `json:"set,omitempty"` // the failing ids, comma separated
	Sent    bool   `json:"sent"`
	Skipped string `json:"skipped,omitempty"` // why nothing was sent
}

type alertState struct {
	Day  string   `json:"day"`
	Sent []string `json:"sent"`
}

// FailingSet returns the sorted ids of the failing selftest checks.
func FailingSet(checks []doctor.Check) []string {
	var ids []string
	for _, c := range checks {
		if c.Status == doctor.Fail && strings.HasPrefix(c.ID, Prefix) {
			ids = append(ids, c.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// Alert sends the message for the failing set unless it already went out today.
func (a Alerter) Alert(ctx context.Context, checks []doctor.Check) (Result, error) {
	ids := FailingSet(checks)
	if len(ids) == 0 {
		return Result{Skipped: "no failing check"}, nil
	}
	set := strings.Join(ids, ",")
	now := time.Now()
	if a.Now != nil {
		now = a.Now()
	}
	now = now.UTC()
	day := now.Format("2006-01-02")
	st := a.load()
	if st.Day != day {
		st = alertState{Day: day}
	}
	for _, s := range st.Sent {
		if s == set {
			return Result{Set: set, Skipped: "already sent today"}, nil
		}
	}
	if len(a.Sinks) == 0 {
		return Result{Set: set, Skipped: "no notifier sink is configured"}, nil
	}
	ev := notify.Event{Kind: notify.KindFailure, Time: now.Format(time.RFC3339), Context: "selftest", Show: "nself ci doctor --selftest",
		Message: message(a.Host, checks, ids)}.Redacted()
	var errs []error
	took := false
	for _, s := range a.Sinks {
		if err := s.Notify(ctx, ev); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.Name(), err))
			continue
		}
		took = true
	}
	if !took {
		return Result{Set: set, Skipped: "no sink took the message"}, errors.Join(errs...)
	}
	st.Sent = append(st.Sent, set)
	return Result{Set: set, Sent: true}, errors.Join(append(errs, a.save(st))...)
}

func message(host string, checks []doctor.Check, ids []string) string {
	names := map[string]string{}
	for _, c := range checks {
		names[c.ID] = c.Name
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = id + " " + names[id]
	}
	where := ""
	if host != "" {
		where = " on " + host
	}
	return fmt.Sprintf("nself-ci selftest%s: %d check(s) failing: %s", where, len(ids), strings.Join(parts, "; "))
}

// load reads the state; an unreadable file is an empty state (the alert is then sent again,
// the safe direction).
func (a Alerter) load() alertState {
	var st alertState
	if b, err := os.ReadFile(a.StatePath); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return st
}

func (a Alerter) save(st alertState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(a.StatePath), 0o700); err != nil {
		return err
	}
	tmp := a.StatePath + ".tmp"
	if err = os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.StatePath)
}
