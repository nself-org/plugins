package main

// Purpose: `nself ci artifacts release <run> --out <dir> [--json]` (P7-CACHE-18,
// Epic CACHE D15): export the published release directory of a run into a fresh
// empty directory, re-verify the copy, and say that it is private operator
// material. This file also wires the publication into the run: it installs
// the run package's ReleasePublisher (the AfterRun hook publishes a PERSONAL
// release run) and the run-id lookup `artifacts verify <run>` uses.
// Inputs:  argv after `release`; env NSELF_CI_HOME, NSELF_CI_REPO,
// NSELF_CI_COORDINATOR_MODE.
// Outputs: the export path and key id, or one v1 JSON envelope; exit 0, 1
// (usage, nothing to export) or 2 (E717 / E719).
// Constraints: V15 only; side-effect class get: it writes only into the
// destination authz.ArtifactGetDest accepts (a fresh, empty directory outside
// any git tree); a release that is failed or awaiting a signature names its
// recovery command and exports nothing; the export holds evidence.json (raw
// evidence with runner ids), which a public project does not publish.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	cirun "github.com/nself-org/plugins/free/ci/internal/run"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func init() {
	registerArtifactsSub("release", artifactsRelease)
	cirun.ReleasePublisher = releasePublisher
	releaseDirOf = publishedDir
}

const releaseUsage = "usage: nself ci artifacts release <run> --out <dir> [--json]"

// publishDeps builds the publication dependencies over an open store.
func publishDeps(st *store.Store, home, root string) (publish.Deps, error) {
	blobs, err := cas.New(records.StoreDir(home))
	if err != nil {
		return publish.Deps{}, err
	}
	env := []string{"NSELF_CI_REPO=" + root, "NSELF_CI_HOME=" + home}
	return publish.Deps{Src: st, Blobs: blobs, Ledger: st, Home: home, Root: root, Capture: report.Capture, Team: teamCoordinator(),
		Signer: &signing.Client{Runner: &signing.FunnelRunner{Home: home}, Src: st, Env: env, Team: teamCoordinator(), Audit: st.AppendAudit}}, nil
}

// releasePublisher is the run package's publication seam.
func releasePublisher(ctx context.Context, r cirun.ReleasePublishRequest) (cirun.ReleaseOutcome, error) {
	home, err := toolsHome()
	if r.Home != "" {
		home, err = r.Home, nil
	}
	if err != nil {
		return cirun.ReleaseOutcome{}, err
	}
	root := r.Root
	if root == "" {
		if root, err = os.Getwd(); err != nil {
			return cirun.ReleaseOutcome{}, err
		}
	}
	d, err := publishDeps(r.Store, home, root)
	if err != nil {
		return cirun.ReleaseOutcome{}, err
	}
	rel, err := publish.Run(ctx, d, r.RunID)
	switch {
	case errors.Is(err, publish.ErrNotRelease):
		return cirun.ReleaseOutcome{}, cirun.ErrNothingToPublish
	case err != nil:
		return cirun.ReleaseOutcome{}, err
	case rel.State == publish.Published:
		rel.Message = "release published to " + rel.Dir + " (export: nself ci artifacts release " + r.RunID + " --out <dir>)"
	}
	return cirun.ReleaseOutcome{State: rel.State, Dir: rel.Dir, Message: rel.Message}, nil
}

// publishedDir maps a run id to its published release directory.
func publishedDir(_ context.Context, run string) (string, bool, error) {
	home, err := toolsHome()
	if err != nil || run == "" || run == "." || run == ".." {
		return "", false, err
	}
	dir := publish.Dir(home, run)
	if st, serr := os.Stat(dir); serr != nil || !st.IsDir() {
		return "", false, nil
	}
	return dir, true, nil
}

func artifactsRelease(args []string) int {
	var out *string
	fs, pos, err := parseArtifacts("release", args, func(fs *flag.FlagSet) {
		out = fs.String("out", "", "fresh empty directory to write the release into")
	})
	if err != nil || len(pos) != 1 || *out == "" {
		fmt.Fprintln(os.Stderr, releaseUsage)
		return 1
	}
	const command = "ci artifacts release"
	asJSON := fs.Lookup("json").Value.String() == "true"
	cwd, err := os.Getwd()
	if err != nil {
		return artifactsExit(err, command, asJSON)
	}
	st, d, err := openPublish(fs)
	if err != nil {
		return artifactsExit(err, command, asJSON)
	}
	defer func() { _ = st.Close() }()
	rel, err := publish.Export(context.Background(), d, pos[0], *out, cwd)
	if err != nil {
		return artifactsExit(err, command, asJSON)
	}
	if asJSON {
		return writeArtifactsJSON(command, rel)
	}
	fmt.Printf("nself-ci: release of run %s exported to %s\nkey %s  manifest %s\n", rel.RunID, rel.Dir, rel.KeyID, shortDigest(rel.ManifestSHA256))
	fmt.Fprintln(os.Stderr, "note: "+rel.Message)
	return 0
}

// openPublish opens the run store and builds the dependencies for the flags.
func openPublish(fs *flag.FlagSet) (*store.Store, publish.Deps, error) {
	home, err := toolsHome()
	if err != nil {
		return nil, publish.Deps{}, err
	}
	root := fs.Lookup("project").Value.String()
	if root == "" {
		if root, err = evidenceRoot(""); err != nil {
			return nil, publish.Deps{}, err
		}
	}
	st, err := cirun.OpenStore(runsStorePath())
	if err != nil {
		return nil, publish.Deps{}, err
	}
	d, err := publishDeps(st, home, root)
	if err != nil {
		_ = st.Close()
		return nil, publish.Deps{}, err
	}
	return st, d, nil
}
