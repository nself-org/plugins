package update

// Purpose: the agent-side half of an upgrade (hq NODE D11): what `nself-ci-agent upgrade` and the protocol `upgrade` message do on
//          the node itself: refuse while a job runs, fetch and verify the release, check the ceiling, swap atomically, keep
//          .prev, and check that the installed file is the verified one.
// Inputs:  LocalOptions: the state dir (slots, coordinators.json), the installed binary path, the target version, a Source.
// Outputs: Result; the binary at BinPath is the new one and BinPath.prev the old one.
// Constraints: a version equal to the kept .prev binary's (recorded next to it as <bin>.prev.version when the swap kept it) is a
//          restore (one rename, no download, allowed as a downgrade because it is the version the node ran before); any other
//          downgrade needs AllowDowngrade; after the swap the installed file is hashed against the signed sha256 and the old binary
//          is restored at once if it differs; the agent never executes the new binary here (os/exec is internal/exec's alone) and never
//          restarts itself: the service manager restarts it (the pull agent exits 0 after a drain) or the next ssh session starts the
//          new binary, and the coordinator's hello decides whether it stays (Engine rolls back with a restore message).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

// LocalOptions is one agent-side upgrade.
type LocalOptions struct {
	StateDir, BinPath string
	// Current is the version of the running agent.
	Current        string
	Version        string
	AllowDowngrade bool
	// AllowRestore lets a request for the version recorded at .prev.version put that binary back without AllowDowngrade. The
	// handler of the coordinator's `upgrade` message sets it (that handler is not wired yet, debt D-1383); an operator typing
	// `nself-ci-agent upgrade` passes AllowDowngrade instead.
	AllowRestore bool
	Platform     lifecycle.Platform
	Source       Source
}

var versionLineRe = regexp.MustCompile(`^nself-ci-agent (\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)(?: |$)`)

// ParseVersionLine reads the version out of `nself-ci-agent version` output.
func ParseVersionLine(out string) (string, bool) {
	line := out
	for i := len(out) - 1; i >= 0; i-- {
		if out[i] == '\n' && i < len(out)-1 {
			line = out[i+1:]
			break
		}
	}
	for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
		line = line[:len(line)-1]
	}
	m := versionLineRe.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// ApplyLocal upgrades the agent installed at o.BinPath.
func ApplyLocal(ctx context.Context, o LocalOptions) (Result, error) {
	res := Result{Node: "local", From: o.Current, To: o.Version, Direction: DirectionOf(o.Current, o.Version)}
	if _, used, err := slots.Status(o.StateDir); err != nil {
		return res, fmt.Errorf("E656: cannot read the slot table: %w", err)
	} else if used > 0 {
		return res, refuse(ReasonRunningJob, "this agent still runs %d job(s); drain it first", used)
	}
	if res.Direction == Same {
		res.Noop = true
		return res, nil
	}
	if kept, _ := os.ReadFile(o.BinPath + PrevVersionSuffix); strings.TrimSpace(string(kept)) == o.Version && (o.AllowRestore || o.AllowDowngrade) {
		recs, rerr := session.ReadCoordinators(o.StateDir)
		if rerr != nil {
			return res, fmt.Errorf("E656: cannot read the recorded coordinators: %w", rerr)
		}
		if cerr := CheckMinAgent(recs, o.Version); cerr != nil {
			return res, cerr
		}
		return res, restoreLocal(o, &res)
	}
	if res.Direction == Downgrade && !o.AllowDowngrade {
		return res, refuse(ReasonDowngrade, "this agent runs %s; %s is a downgrade", o.Current, o.Version)
	}
	prep, err := o.Source.Prepare(ctx, o.Platform)
	if err != nil {
		return res, err
	}
	defer func() {
		if prep.Cleanup != nil {
			prep.Cleanup()
		}
	}()
	recs, err := session.ReadCoordinators(o.StateDir)
	if err != nil {
		return res, fmt.Errorf("E656: cannot read the recorded coordinators: %w", err)
	}
	if err = CheckCeiling(recs, prep.Manifest); err != nil {
		return res, err
	}
	// Look again right before the swap: another coordinator may have leased a job during the download.
	if _, used, serr := slots.Status(o.StateDir); serr != nil || used > 0 {
		return res, refuse(ReasonRunningJob, "a job started during the download; nothing was installed")
	}
	if err = Swap(o.BinPath, prep.Path); err != nil {
		return res, err
	}
	if got, herr := fileSHA256(o.BinPath); herr != nil || got != prep.Manifest.SHA256 {
		_ = os.Remove(o.BinPath + PrevVersionSuffix)
		if rerr := Restore(o.BinPath); rerr != nil {
			return res, fmt.Errorf("E656: the installed file is not the verified one and the previous agent could not be restored: %v", rerr)
		}
		res.RolledBack = true
		return res, fmt.Errorf("E656: the installed agent is not the verified file; the previous agent is back")
	}
	if _, serr := os.Lstat(o.BinPath + PrevSuffix); serr == nil {
		_ = os.WriteFile(o.BinPath+PrevVersionSuffix, []byte(o.Current+"\n"), 0o600)
	}
	return res, nil
}

// restoreLocal puts the kept previous binary back (the version asked for is the version it was recorded as).
func restoreLocal(o LocalOptions, res *Result) error {
	if err := Restore(o.BinPath); err != nil {
		return err
	}
	_ = os.Remove(o.BinPath + PrevVersionSuffix)
	_ = os.Remove(o.BinPath + NewSuffix)
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
