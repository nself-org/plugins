package main

// Purpose: `nself ci artifacts sign <run> --coordinator <url> --out <dir>` (P7-CACHE-17, Epic CACHE
// D15, TEAM path): a TEAM coordinator holds secret references only, so the release is signed here, on
// the operator's machine. The command fetches the run's release inputs and blobs through the
// coordinator client (pinned root, user certificate or scoped token), re-hashes every blob, rebuilds
// the provenance and manifest with the publication code, signs them with the operator's own release
// key and writes the verified release directory to --out. It registers itself as a publish sign
// source, so cmd/artifacts_sign.go is unchanged.
// Inputs:  --coordinator https://host[:port] (env NSELF_CI_COORDINATOR and user config ci.* also
// apply to the credential files), --out <fresh empty dir>, --project <checkout of the repository>.
// Outputs: the release directory and key id (or one v1 JSON envelope); exit 0, 1 (usage, nothing to
// publish) or 2 (E664 / E687 / E713 / E717 / E719).
// Constraints: V15 only; side-effect class write (the --out directory; the operator's CI home is used
// as a private work area that is removed); only GET requests reach the coordinator; the key is
// resolved by this process only and never leaves it; nothing is uploaded back.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/coordclient"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	"github.com/nself-org/plugins/free/ci/internal/release/remote"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/signing"
)

func init() {
	publish.RegisterSignSource("coordinator", func(fs *flag.FlagSet) {
		fs.String("coordinator", "", "sign a TEAM coordinator's release here, on this machine (https://host[:port]; needs --out)")
		fs.String("out", "", "fresh empty directory to write the release into (with --coordinator)")
	}, signRemote)
}

// signRemote is the `--coordinator` sign source.
func signRemote(ctx context.Context, run string, fs *flag.FlagSet) (publish.Release, error) {
	root := fs.Lookup("project").Value.String()
	var err error
	if root == "" {
		if root, err = evidenceRoot(""); err != nil {
			return publish.Release{}, err
		}
	}
	home, err := toolsHome()
	if err != nil {
		return publish.Release{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return publish.Release{}, err
	}
	target, warns, err := coordclient.Resolve(coordclient.Inputs{Flag: fs.Lookup("coordinator").Value.String(), RepoRoot: root})
	for _, w := range warns {
		fmt.Fprintln(os.Stderr, w)
	}
	if err != nil {
		return publish.Release{}, err
	}
	if target.Kind != coordclient.Remote {
		return publish.Release{}, clientapi.Failf(clientapi.ReasonBadRequest, "--coordinator needs a TEAM coordinator address")
	}
	client, err := coordclient.New(target)
	if err != nil {
		return publish.Release{}, err
	}
	rel, err := remote.Sign(ctx, remote.Params{Fetcher: client, Run: run, Out: fs.Lookup("out").Value.String(), Cwd: cwd, Root: root, OperatorHome: home,
		Capture: report.Capture, Coordinator: target.URL,
		NewSigner: func(src signing.Source, blobs *cas.Store, project string) publish.Signer {
			return remote.NewLocalSigner(src, blobs, signerResolver(root), project)
		}})
	if errors.Is(err, publish.ErrNotRelease) {
		return rel, err
	}
	if err == nil {
		fmt.Fprintln(os.Stderr, "note: "+publish.PrivateNotice)
	}
	return rel, err
}
