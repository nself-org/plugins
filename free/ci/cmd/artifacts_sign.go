package main

// Purpose: `nself ci artifacts sign <run> [--json]` (P7-CACHE-18, Epic CACHE
// D15): publish the release of a run, or complete a publication that failed
// or was killed after the evidence was recorded. It repeats the publication
// steps (read the stored facts, build, sign in the signer, self-verify,
// record) and, because every byte is deterministic, a second call writes
// byte-identical files and signs nothing when the release is intact.
// Inputs:  argv after `sign`; env NSELF_CI_HOME, NSELF_CI_REPO; the flags of
// every source registered with publish.RegisterSignSource (P7-CACHE-17 adds
// `--coordinator <url>`); at most one source flag may be set, and a source's companion
// flag (--out of --coordinator) is refused without it.
// Outputs: the release directory and key id, or one v1 JSON envelope; exit 0,
// 1 (usage, nothing to publish) or 2 (E717 / E719).
// Constraints: V15 only; side-effect class write (the release directory and
// the release ledger row, never the evidence); on a TEAM coordinator the local
// source records awaiting-signature and resolves no secret.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	"github.com/nself-org/plugins/free/ci/internal/signing"
)

func init() { registerArtifactsSub("sign", artifactsSign) }

const signUsage = "usage: nself ci artifacts sign <run> [--json] [--project <dir>] [source flags, for example --coordinator <url>]"

func artifactsSign(args []string) int {
	srcs := publish.SignSources()
	fs, pos, err := parseArtifacts("sign", args, func(fs *flag.FlagSet) {
		for _, s := range srcs {
			if s.Flags != nil {
				s.Flags(fs)
			}
		}
	})
	if err != nil || len(pos) != 1 {
		fmt.Fprintln(os.Stderr, signUsage)
		return 1
	}
	const command = "ci artifacts sign"
	asJSON := fs.Lookup("json").Value.String() == "true"
	var chosen *publish.SignSource
	for i, s := range srcs {
		if f := fs.Lookup(s.Name); f != nil && f.Value.String() != "" && f.Value.String() != f.DefValue {
			if chosen != nil {
				fmt.Fprintln(os.Stderr, "nself ci artifacts sign: use one source flag at most")
				return 1
			}
			chosen = &srcs[i]
		}
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	for _, s := range srcs {
		if chosen != nil && s.Name == chosen.Name {
			continue
		}
		for _, c := range s.Companions {
			if set[c] {
				fmt.Fprintf(os.Stderr, "nself ci artifacts sign: --%s needs --%s\n%s\n", c, s.Name, signUsage)
				return 1
			}
		}
	}
	ctx := context.Background()
	var rel publish.Release
	if chosen != nil {
		rel, err = chosen.Run(ctx, pos[0], fs)
	} else {
		var closeStore func()
		if rel, closeStore, err = signLocal(ctx, fs, pos[0]); closeStore != nil {
			defer closeStore()
		}
	}
	if errors.Is(err, publish.ErrNotRelease) {
		msg := "run " + pos[0] + " has no passing release job: nothing to publish"
		if asJSON {
			_ = output.WriteError(os.Stdout, command, output.ErrorDetail{Code: signing.CodeSigning, Message: msg, ExitCode: output.ExitUser, Class: output.ClassUser}, nil)
			return output.ExitUser
		}
		fmt.Fprintln(os.Stderr, "nself ci artifacts sign: "+msg)
		return output.ExitUser
	}
	if err != nil {
		return artifactsExit(err, command, asJSON)
	}
	if asJSON {
		return writeArtifactsJSON(command, rel)
	}
	if rel.State == publish.Published {
		fmt.Printf("nself-ci: release of run %s published to %s\nkey %s  manifest %s\n", rel.RunID, rel.Dir, rel.KeyID, shortDigest(rel.ManifestSHA256))
		return 0
	}
	fmt.Printf("nself-ci: release of run %s is %s\n%s\n", rel.RunID, rel.State, rel.Message)
	return 0
}

// signLocal publishes with the signer on this machine.
func signLocal(ctx context.Context, fs *flag.FlagSet, run string) (publish.Release, func(), error) {
	st, d, err := openPublish(fs)
	if err != nil {
		return publish.Release{}, nil, err
	}
	rel, err := publish.Run(ctx, d, run)
	return rel, func() { _ = st.Close() }, err
}
