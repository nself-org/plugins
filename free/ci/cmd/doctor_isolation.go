package main

// Purpose: the isolation section of `nself ci doctor` (P7-TRUST-25, R-DX-25). One check per isolation level
// (isolation.container, isolation.sandboxed-container, isolation.network, isolation.hygiene, isolation.vm and, on
// Linux, isolation.ephemeral-vm) saying when this machine last proved the level, from the stored evidence of the
// isolation-levels drill (scripts/drill/isolation-levels). It registers itself in the doctor section registry
// (P7-CI-50), so internal/doctor is not edited for it.
// Inputs:  the machine's run store (NSELF_CI_HOME, default ~/.nself/ci/state.db); it is only read, and never created.
// Outputs: checks that always pass (an unverified level is the honest state of a host that never ran the drill, not a
// fault); `verified_at` is an RFC 3339 time or null; Linux vm and ephemeral-vm read "unavailable (no KVM host, D-0120)".
// A store that cannot be read is one warning, and no level then reads verified.
// Constraints: no write, no store creation; a level is verified only by a stored passing check.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/isolation/lastverified"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// isolationGOOS is the host OS the section reports for; tests replace it.
var isolationGOOS = runtime.GOOS

const (
	isolationDrillHint = "Run the isolation-levels drill on this host: nself ci --pipeline drills --job drill-isolation-levels"
	isolationStoreHint = "Check the state directory (NSELF_CI_HOME) and that no other nself-ci process holds it"
)

func init() {
	doctor.RegisterSection(doctor.Section{Name: "isolation", Selector: doctor.SelectAlways, Run: isolationChecks})
}

func isolationChecks(ctx context.Context, env doctor.Env) []doctor.Check {
	var src lastverified.Source
	storeErr := error(nil)
	if path := isolationStorePath(env); path != "" {
		if _, err := os.Stat(path); err == nil {
			st, openErr := cirun.OpenStore(path)
			if openErr != nil {
				storeErr = openErr
			} else {
				defer func() { _ = st.Close() }()
				src = st
			}
		}
	}
	records, readErr := lastverified.Read(ctx, src, isolationGOOS)
	if storeErr == nil {
		storeErr = readErr
	}
	var out []doctor.Check
	if storeErr != nil {
		out = append(out, doctor.Check{
			ID: "isolation.record", Name: "isolation last-verified record", Status: doctor.Warn, Subject: "isolation",
			Message: "the isolation last-verified record could not be read: " + storeErr.Error(), Remediation: isolationStoreHint,
		})
	}
	for _, r := range records {
		c := doctor.Check{
			ID: r.Level.CheckID(), Name: "isolation " + string(r.Level), Status: doctor.Pass, Subject: string(r.Level),
			Message: r.Message(), HasVerifiedAt: true,
		}
		switch r.Status {
		case lastverified.Verified:
			at := r.VerifiedAt.UTC().Format(time.RFC3339)
			c.VerifiedAt = &at
		case lastverified.Unverified:
			c.Remediation = isolationDrillHint
		}
		out = append(out, c)
	}
	return out
}

// isolationStorePath is the state database, from NSELF_CI_HOME or the user's home; "" when neither is known.
func isolationStorePath(env doctor.Env) string {
	home := env.Getenv("NSELF_CI_HOME")
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		home = filepath.Join(user, ".nself", "ci")
	}
	return filepath.Join(home, "state.db")
}
