// Package lastverified answers, per isolation level, "when did this host last prove it?" (P7-TRUST-25, R-DX-25).
//
// Purpose: isolation evidence rots (a runtime update, a kernel change), so `nself ci doctor` shows the date of the
// newest passing check of each level instead of trusting that a level once worked. The date comes from the stored
// evidence of the isolation-levels drill (scripts/drill/isolation-levels); nothing here writes.
// Inputs:  a Source (the run store) and the host OS.
// Outputs: one Record per level: verified (date, revision, host class), unverified, or unavailable.
// Constraints: a level is verified only by a passing nested check found in stored evidence; a source error or a level
// with no passing check reads unverified, never verified; Linux vm and ephemeral-vm are always unavailable (no KVM
// host, D-0120).
package lastverified

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Job is the id of the drill job whose evidence is read, and PassedPrefix starts the excerpt line that lists the
// nested checks that passed (internal/drill writes it).
const (
	Job          = "drill-isolation-levels"
	PassedPrefix = "passed: "
)

// Level is one isolation level; the nested check id is "isolation."+Level.
type Level string

// The levels, in report order. Network scopes and workspace hygiene are not isolation values of a job but are
// proven by the same drill, so they are levels of this record.
const (
	Container          Level = "container"
	SandboxedContainer Level = "sandboxed-container"
	Network            Level = "network"
	Hygiene            Level = "hygiene"
	VM                 Level = "vm"
	EphemeralVM        Level = "ephemeral-vm"
)

// CheckID is the nested check id the drill reports for the level.
func (l Level) CheckID() string { return "isolation." + string(l) }

// Status is the state of one level on this host.
type Status string

// The three states.
const (
	Verified    Status = "verified"
	Unverified  Status = "unverified"
	Unavailable Status = "unavailable"
)

// UnavailableNote is why Linux has no vm level.
const UnavailableNote = "no KVM host, D-0120"

// Record is the last-verified state of one level.
type Record struct {
	Level      Level
	Status     Status
	VerifiedAt time.Time // set when Status is Verified
	Revision   string
	HostClass  string // the runner platform of the verifying run, for example linux/amd64
}

// Source supplies stored passes; *store.Store implements it.
type Source interface {
	LastVerified(ctx context.Context, jobID, passedPrefix string, ids []string) (map[string]store.Verification, error)
}

// Levels returns the levels reported on a host with the given GOOS: ephemeral-vm is a Linux-only line.
func Levels(goos string) []Level {
	out := []Level{Container, SandboxedContainer, Network, Hygiene, VM}
	if goos == "linux" {
		out = append(out, EphemeralVM)
	}
	return out
}

// Read builds one Record per level for a host. src may be nil (no store yet): every level but the unavailable ones
// reads unverified. err is the source error, if any; the records are still complete and honest.
func Read(ctx context.Context, src Source, goos string) ([]Record, error) {
	levels := Levels(goos)
	ids := make([]string, 0, len(levels))
	for _, l := range levels {
		ids = append(ids, l.CheckID())
	}
	var found map[string]store.Verification
	var err error
	if src != nil {
		found, err = src.LastVerified(ctx, Job, PassedPrefix, ids)
	}
	out := make([]Record, 0, len(levels))
	for _, l := range levels {
		r := Record{Level: l, Status: Unverified}
		if v, ok := found[l.CheckID()]; ok && err == nil {
			r = Record{Level: l, Status: Verified, VerifiedAt: v.FinishedAt, Revision: v.Revision, HostClass: v.Platform}
		}
		if goos == "linux" && (l == VM || l == EphemeralVM) {
			r = Record{Level: l, Status: Unavailable}
		}
		out = append(out, r)
	}
	return out, err
}

// Message is the one-line text of a record.
func (r Record) Message() string {
	switch r.Status {
	case Verified:
		msg := fmt.Sprintf("verified %s on %s", r.VerifiedAt.UTC().Format("2006-01-02"), firstNonEmpty(r.HostClass, "unknown host class"))
		if len(r.Revision) >= 7 {
			msg += " (revision " + r.Revision[:7] + ")"
		}
		return msg
	case Unavailable:
		return "unavailable (" + UnavailableNote + ")"
	}
	return "unverified since never"
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
