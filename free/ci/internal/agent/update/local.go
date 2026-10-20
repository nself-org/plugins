package update

// Purpose: the agent-side half of an upgrade (hq NODE D11): what `nself-ci-agent upgrade` and the protocol `upgrade` message do on
//          the node itself: refuse while a job runs, fetch and verify the release, check the ceiling, swap atomically, keep
//          .prev, and check that the new binary runs.
// Inputs:  LocalOptions: the state dir (slots, coordinators.json), the installed binary path, the target version, a Source.
// Outputs: Result; the binary at BinPath is the new one and BinPath.prev the old one.
// Constraints: a version equal to the kept .prev binary's is a restore (one rename, no download, allowed as a downgrade because it
//          is the version the node ran before); any other downgrade needs AllowDowngrade; the staged binary must print the target
//          version before it replaces anything, and the installed one again afterwards (else the old one is restored at once); the
//          agent never restarts itself here: the service manager restarts it (the pull agent exits 0 after a drain) or the next
//          ssh session starts the new binary.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"time"

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
	Platform       lifecycle.Platform
	Source         Source
	// Smoke runs `<bin> version` and returns the version it printed; nil runs the real binary.
	Smoke func(ctx context.Context, bin string) (string, error)
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

func runVersion(ctx context.Context, bin string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, bin, "version").Output()
	if err != nil {
		return "", fmt.Errorf("E656: %s did not run: %w", bin, err)
	}
	v, ok := ParseVersionLine(string(out))
	if !ok {
		return "", fmt.Errorf("E656: %s printed an unexpected version line", bin)
	}
	return v, nil
}

// ApplyLocal upgrades the agent installed at o.BinPath.
func ApplyLocal(ctx context.Context, o LocalOptions) (Result, error) {
	res := Result{Node: "local", From: o.Current, To: o.Version, Direction: DirectionOf(o.Current, o.Version)}
	smoke := o.Smoke
	if smoke == nil {
		smoke = runVersion
	}
	if _, used, err := slots.Status(o.StateDir); err != nil {
		return res, fmt.Errorf("E656: cannot read the slot table: %w", err)
	} else if used > 0 {
		return res, refuse(ReasonRunningJob, "this agent still runs %d job(s); drain it first", used)
	}
	if res.Direction == Same {
		res.Noop = true
		return res, nil
	}
	if prevVersion, err := smoke(ctx, o.BinPath+PrevSuffix); err == nil && prevVersion == o.Version {
		return res, restoreLocal(ctx, o, smoke, &res)
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
	if got, serr := smoke(ctx, prep.Path); serr != nil || got != o.Version {
		return res, fmt.Errorf("E656: the verified release does not run here as %s (%v)", o.Version, serr)
	}
	if err = Swap(o.BinPath, prep.Path); err != nil {
		return res, err
	}
	if got, serr := smoke(ctx, o.BinPath); serr != nil || got != o.Version {
		if rerr := Restore(o.BinPath); rerr != nil {
			return res, fmt.Errorf("E656: the new agent does not run and the previous one could not be restored: %v", rerr)
		}
		res.RolledBack = true
		return res, fmt.Errorf("E656: the new agent did not run as %s (%v); the previous agent is back", o.Version, serr)
	}
	return res, nil
}

// restoreLocal puts the kept previous binary back (the version asked for is the version it reports).
func restoreLocal(ctx context.Context, o LocalOptions, smoke func(context.Context, string) (string, error), res *Result) error {
	if err := Restore(o.BinPath); err != nil {
		return err
	}
	if got, err := smoke(ctx, o.BinPath); err != nil || got != o.Version {
		return fmt.Errorf("E656: the restored agent did not run as %s (%v)", o.Version, err)
	}
	_ = os.Remove(o.BinPath + NewSuffix)
	res.Direction = Downgrade
	return nil
}
