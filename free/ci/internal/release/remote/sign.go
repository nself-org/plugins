package remote

// Purpose: Sign, `nself ci artifacts sign <run> --coordinator <url>` (Epic CACHE D15, TEAM path):
// fetch and re-hash the release inputs, rebuild the provenance and the manifest with the publication
// code (internal/release/publish), sign them locally with the operator's own release key, verify
// the result and write the release directory to --out.
// Inputs:  Params: the coordinator client, the run, the destination, the operator's checkout and
// CI home, the signer factory.
// Outputs: publish.Release with Dir set to the written directory; ErrNotRelease passed through.
// Constraints: nothing is uploaded back; the coordinator receives GET requests only and never a key,
// a signature or a secret value; the work happens in a private directory under the operator's CI
// home that is removed on every path; the operator's trust files are copied in so a rotated key still
// verifies; the run's repository must be the operator's checkout (checked before the key is
// resolved); a failed fetch, re-hash, check, signature or verification writes nothing to --out.

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	"github.com/nself-org/plugins/free/ci/internal/signing"
)

const (
	workPrefix = ".team-sign-"
	staleAfter = 6 * time.Hour
)

// heartbeat is how often a running sign touches its work directory (a variable so a test can speed it up).
var heartbeat = 5 * time.Minute

// Params are the inputs of Sign.
type Params struct {
	Fetcher Fetcher
	Run     string
	// Out is the destination (a fresh empty directory outside any git tree); Cwd resolves a relative Out.
	Out, Cwd string
	// Root is the operator's checkout of the run's repository; OperatorHome is NSELF_CI_HOME.
	Root, OperatorHome string
	// NewSigner builds the operator's signer over the fetched facts and blobs.
	NewSigner func(src signing.Source, blobs *cas.Store, project string) publish.Signer
	Capture   evidence.Capture
	// Verify replaces the release verifier in tests.
	Verify publish.VerifyFunc
	// Coordinator is the URL shown in the recovery command.
	Coordinator string
}

// Sign runs the TEAM signing.
func Sign(ctx context.Context, p Params) (publish.Release, error) {
	if !ValidRunID(p.Run) || p.Fetcher == nil || p.NewSigner == nil || p.Root == "" || p.OperatorHome == "" {
		return publish.Release{}, fail(signing.CodeSigning, signing.ReasonPayloadRejected, "the signing request is incomplete")
	}
	if _, err := authz.ArtifactGetDest(p.Out, p.Cwd); err != nil {
		return publish.Release{}, err // before the key or the network is touched
	}
	if err := os.MkdirAll(p.OperatorHome, 0o700); err != nil {
		return publish.Release{}, fail(signing.CodeSigning, signing.ReasonSignerFailed, "the CI home could not be created")
	}
	sweepStale(p.OperatorHome, time.Now())
	home, err := os.MkdirTemp(p.OperatorHome, workPrefix)
	if err != nil {
		return publish.Release{}, fail(signing.CodeSigning, signing.ReasonSignerFailed, "a private work directory could not be created")
	}
	defer func() { _ = os.RemoveAll(home) }()
	if err = copyTrust(p.OperatorHome, home); err != nil {
		return publish.Release{}, fail(signing.CodeSigning, signing.ReasonSignerFailed, "the operator's trust files could not be read")
	}
	stop := keepAlive(home)
	defer stop()
	in, err := FetchInputs(ctx, p.Fetcher, p.Run)
	if err != nil {
		return publish.Release{}, err
	}
	if err = sameRepo(ctx, p, in); err != nil { // before any blob is downloaded and before the key is touched
		return publish.Release{}, err
	}
	snap, blobs, err := FetchBlobs(ctx, p.Fetcher, in, filepath.Join(home, "blobs"))
	if err != nil {
		return publish.Release{}, err
	}
	d := publish.Deps{Src: snap, Blobs: blobs, Ledger: snap, Signer: p.NewSigner(snap, blobs, snap.Inputs().Pipeline.Project), Home: home,
		Root: p.Root, Capture: p.Capture, Verify: p.Verify}
	rel, err := publish.Run(ctx, d, p.Run)
	if err != nil {
		return rel, p.recover(err)
	}
	if rel.State != publish.Published {
		return rel, fail(signing.CodeSigning, signing.ReasonSignerFailed, "the release was not published (%s)", rel.State)
	}
	rel, err = publish.Export(ctx, d, p.Run, p.Out, p.Cwd)
	if err != nil {
		return rel, p.recover(err)
	}
	return rel, nil
}

// sameRepo refuses to use the key for a run of another repository than the operator's checkout.
func sameRepo(ctx context.Context, p Params, in Inputs) error {
	var ev model.Evidence
	if err := jsonUnmarshal(in.Evidence, &ev); err != nil || ev.RepoID == "" {
		return fail(signing.CodeSigning, signing.ReasonPayloadRejected, "the stored evidence does not name its repository")
	}
	capture := p.Capture
	if capture == nil {
		return fail(signing.CodeSigning, signing.ReasonSignerFailed, "no git runner is configured")
	}
	if id, ok := evidence.RepoID(ctx, capture, p.Root); !ok || id != ev.RepoID {
		return fail(signing.CodeSigning, signing.ReasonPayloadRejected, "run %s belongs to repository %s, which is not the checkout at %s: run this command inside that repository (or pass --project)", p.Run, shown(ev.RepoID), p.Root)
	}
	return nil
}

// keepAlive touches dir every few minutes while a sign runs, so the sweep of another sign never takes
// a long download for an abandoned one. The returned function stops it.
func keepAlive(dir string) func() {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(heartbeat)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case now := <-t.C:
				_ = os.Chtimes(dir, now, now)
			}
		}
	}()
	return func() { close(done); wg.Wait() }
}

// shown makes text from the coordinator safe to print: control characters out, cut at 80 runes.
func shown(s string) string {
	r := []rune(strings.Map(func(c rune) rune {
		if unicode.IsControl(c) {
			return '?'
		}
		return c
	}, s))
	if len(r) > 80 {
		r = r[:80]
	}
	return string(r)
}

// sweepStale removes private work directories an earlier, killed run left behind.
func sweepStale(operatorHome string, now time.Time) {
	entries, err := os.ReadDir(operatorHome)
	if err != nil {
		return
	}
	for _, e := range entries {
		if info, ierr := e.Info(); ierr == nil && e.IsDir() && strings.HasPrefix(e.Name(), workPrefix) && now.Sub(info.ModTime()) > staleAfter {
			_ = os.RemoveAll(filepath.Join(operatorHome, e.Name()))
		}
	}
}

// recover points the recovery hint of a publication failure at the TEAM command.
func (p Params) recover(err error) error {
	var pe *publish.Error
	if errors.As(err, &pe) && p.Coordinator != "" {
		out := *pe
		out.Msg = strings.ReplaceAll(out.Msg, "`"+publish.RecoveryCommand(p.Run)+"`", "`"+publish.RecoveryCommand(p.Run)+" --coordinator "+p.Coordinator+" --out <dir>`")
		return &out
	}
	return err
}

// copyTrust copies the operator's ci-release trust files into the private home.
func copyTrust(from, to string) error {
	for _, name := range []string{"ci-release.keys", "ci-release.revoked"} {
		src := filepath.Join(from, "trust", name)
		st, err := os.Lstat(src)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil || !st.Mode().IsRegular() || st.Size() > 1<<20 {
			return errors.New("trust file is not a small regular file")
		}
		b, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Join(to, "trust"), 0o700); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(to, "trust", name), b, 0o600); err != nil {
			return err
		}
	}
	return nil
}
