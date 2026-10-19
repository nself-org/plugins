package main

// Purpose: `nself ci artifacts verify <dir|manifest|run> [--keys <file>]
// [--repo <dir>] [--json]` (P7-CACHE-09, Epic CACHE D16): check a release
// directory with internal/release/verify and say whether it is the release it
// claims to be.
// Inputs:  a release directory, its release-manifest.dsse.json, or a run id
// (resolved by the publication hook P7-CACHE-18 registers); env NSELF_CI_HOME
// (operator trust files in $NSELF_CI_HOME/trust/ci-release.{keys,revoked}),
// NSELF_CI_REPO (the repository the release came from; default the working
// directory). --keys replaces the operator keys file.
// Outputs: a summary or one v1 envelope (--json) whose data always carries
// `slsa_build_level: "L1"`; exit 0 verified, 11 invalid (the reason key names
// the check), 1 evidence failing or quarantined.
// Constraints: V15 only; side-effect class read: it writes nothing. The trust
// roots are the operator files and the protected default branch at the pinned
// tip (one `git fetch` of the default branch, as the policy resolver does);
// never the working tree or the released revision.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/release/verify"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// Seams: tests replace them; later Tickets register the real ones.
var (
	// releasePin returns the pinned tip of the protected default branch ("" when none).
	releasePin = resolveReleasePin
	// releaseDirOf maps a run id to its release directory (P7-CACHE-18 registers it).
	releaseDirOf func(ctx context.Context, run string) (dir string, ok bool, err error)
	// releaseQuarantined reports evidence quarantine (P7-TRUST-20 registers it).
	releaseQuarantined func(ctx context.Context, ev model.Evidence) (bool, error)
)

func init() { registerArtifactsSub("verify", artifactsVerify) }

const verifyUsage = "usage: nself ci artifacts verify <dir|manifest|run> [--keys <file>] [--repo <dir>] [--json]"

func artifactsVerify(args []string) int {
	var keys, repo *string
	fs, pos, err := parseArtifacts("verify", args, func(fs *flag.FlagSet) {
		keys = fs.String("keys", "", "ci-release keys file to trust instead of the operator-scope file")
		repo = fs.String("repo", "", "repository the release came from (env NSELF_CI_REPO; default: current directory)")
	})
	if err != nil || len(pos) != 1 {
		fmt.Fprintln(os.Stderr, verifyUsage)
		return 1
	}
	const command = "ci artifacts verify"
	asJSON := fs.Lookup("json").Value.String() == "true"
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	opt := verify.Options{Capture: report.Capture, Quarantined: releaseQuarantined}
	root, err := evidenceRoot(*repo)
	if err != nil {
		return artifactsExit(err, command, asJSON)
	}
	opt.Root = root
	if home, herr := toolsHome(); herr == nil {
		opt.OperatorKeys = filepath.Join(home, "trust", "ci-release.keys")
		opt.OperatorRevoked = filepath.Join(home, "trust", "ci-release.revoked")
	}
	if *keys != "" {
		opt.OperatorKeys = *keys
	}
	var notes []string
	if opt.Pin, err = releasePin(ctx, root); err != nil {
		notes = append(notes, "the protected default branch could not be read, so only operator keys are trusted: "+err.Error())
	}
	dir, err := releaseDir(ctx, pos[0])
	if err != nil {
		return artifactsExit(err, command, asJSON)
	}
	res := verify.Release(ctx, dir, opt)
	res.Warnings = append(res.Warnings, notes...)
	return printVerify(command, asJSON, res)
}

// releaseDir resolves the input: a directory, the manifest file inside one, or a run id.
func releaseDir(ctx context.Context, arg string) (string, error) {
	st, err := os.Stat(arg)
	switch {
	case err == nil && st.IsDir():
		return arg, nil
	case err == nil && st.Mode().IsRegular() && filepath.Base(arg) == verify.ManifestFile:
		return filepath.Dir(arg), nil
	case err == nil:
		return "", fmt.Errorf("%s is neither a release directory nor a %s", arg, verify.ManifestFile)
	case releaseDirOf == nil:
		return arg, nil // not a path and no run lookup: Release reports release_missing
	}
	dir, ok, lerr := releaseDirOf(ctx, arg)
	if lerr != nil {
		return "", lerr
	}
	if !ok {
		return arg, nil
	}
	return dir, nil
}

func printVerify(command string, asJSON bool, res verify.Result) int {
	var meta *output.Meta
	if len(res.Warnings) > 0 {
		meta = &output.Meta{Warnings: res.Warnings}
	}
	if asJSON {
		_ = output.WriteData(os.Stdout, command, res, meta)
		return res.Exit
	}
	for _, w := range res.Warnings {
		fmt.Fprintln(os.Stderr, "note: "+w)
	}
	if !res.Verified {
		fmt.Fprintf(os.Stderr, "nself-ci: release NOT verified: %s %s: %s\n", res.Code, res.Reason, res.Message)
		return res.Exit
	}
	fmt.Printf("nself-ci: release verified (SLSA %s)\nrevision %.12s  run %s  key %s  artifacts %d\n", "Build "+res.SLSABuildLevel, res.Revision, res.RunID, res.KeyID, res.Artifacts)
	return res.Exit
}

// resolveReleasePin reads the pinned tip of the protected default branch the
// way the policy resolver does.
func resolveReleasePin(ctx context.Context, root string) (string, error) {
	r := policy.Resolver{Run: policy.Runner(report.Capture)}
	res, err := r.Resolve(ctx, policy.Repo{Root: root}, policy.Revision{})
	if err != nil {
		return "", err
	}
	return res.PinnedCommit, nil
}
