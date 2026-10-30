package selftest

// Purpose: the tidy step that needs no store: the orphan job-root sweep (TRUST-09).
// The retention and cache steps need the run store and are built by the command.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/hygiene"
)

// OrphanStep lists the job roots OrphanRoots finds and removes them with the hygiene sweep.
// base is <home>/work; home holds the process markers.
func OrphanStep(base, home string, now func() time.Time) Step {
	return Step{
		Name: "job roots",
		Plan: func(context.Context) ([]Item, error) {
			names, err := OrphanRoots(base, OrphanMinAge, now())
			items := make([]Item, len(names))
			for i, n := range names {
				items[i] = Item{Step: "job roots", ID: n}
			}
			return items, err
		},
		Apply: func(ctx context.Context, _ []Item) error {
			rep, err := (&hygiene.Host{Base: base, Home: home, MinAge: OrphanMinAge}).SweepOrphans(ctx, nil)
			if err != nil {
				return err
			}
			var failed []string
			for what, ferr := range rep.Failed {
				failed = append(failed, what+": "+ferr.Error())
			}
			if len(failed) > 0 {
				return fmt.Errorf("%s", strings.Join(failed, "; "))
			}
			return nil
		},
	}
}
