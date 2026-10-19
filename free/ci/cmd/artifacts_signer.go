package main

// Purpose: the hidden `nself-ci artifacts signer` process (P7-CACHE-07, Epic
// CACHE D12): the only place the release key is ever resolved. The
// coordinator starts it through internal/exec after the run, outside every
// job's process group, namespace and container.
// Inputs:  one mode flag: --stdio (one request on stdin), --public, --init,
// --rotate; env NSELF_CI_HOME, NSELF_CI_REPO (the project) and NSELF_V15=1.
// Outputs: one JSON document on stdout (the response, or {"error":{...}});
// exit 0, 1 (usage) or 2 (refused or failed).
// Constraints: refuses on a TEAM coordinator (NSELF_CI_COORDINATOR_MODE=team); V15 only and hidden (not in the usage text); a request is
// re-checked against the run store before anything is signed; the key is
// resolved by reference, used in memory and zeroed; nothing but envelopes or
// the public trust line is printed; a key never reaches argv, stdout, a file
// or a child's environment.

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/signing"
)

// signerIn and signerOut replace stdin and stdout in tests (nil: the process's own).
var (
	signerIn  io.Reader
	signerOut io.Writer
)

func signerStdin() io.Reader {
	if signerIn != nil {
		return signerIn
	}
	return os.Stdin
}

func signerStdout() io.Writer {
	if signerOut != nil {
		return signerOut
	}
	return os.Stdout
}

func init() {
	registerArtifactsSub("signer", artifactsSigner)
}

func artifactsSigner(args []string) int {
	fs := flag.NewFlagSet("artifacts signer", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	stdio, public := fs.Bool("stdio", false, ""), fs.Bool("public", false, "")
	initKey, rotate := fs.Bool("init", false, ""), fs.Bool("rotate", false, "")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || countTrue(*stdio, *public, *initKey, *rotate) != 1 {
		fmt.Fprintln(os.Stderr, "nself ci artifacts signer is started by the coordinator; it takes one of --stdio, --public, --init, --rotate")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), signerTimeout)
	defer cancel()
	if teamCoordinator() {
		return signerFail(signing.Failf(signing.ReasonSignerFailed, "a TEAM coordinator never holds the release key: sign on the operator's machine"))
	}
	root, err := evidenceRoot("")
	if err != nil || root == "" {
		return signerFail(signing.Failf(signing.ReasonSignerFailed, "the project root is unknown"))
	}
	switch {
	case *stdio:
		return signerRequest(ctx, root)
	case *public:
		return signerKey(signing.Public(ctx, signerResolver(root), signing.KeyAttempt))
	}
	st, err := signerStore(root)
	if err != nil {
		return signerFail(signing.Failf(signing.ReasonSignerFailed, "the user home is unknown"))
	}
	if *initKey {
		return signerKey(signing.Init(ctx, signerResolver(root), st))
	}
	return signerKey(signing.Rotate(ctx, signerResolver(root), st))
}

// signerTimeout bounds one signer call (the key lock wait included).
const signerTimeout = 50 * time.Second

// teamCoordinator reports whether this process runs as a TEAM coordinator,
// which holds secret references only (Epic X8) and so never resolves the key.
func teamCoordinator() bool { return os.Getenv("NSELF_CI_COORDINATOR_MODE") == "team" }

func countTrue(b ...bool) (n int) {
	for _, v := range b {
		if v {
			n++
		}
	}
	return n
}

// signerResolver resolves release secrets through the exec funnel and the cli.
// The funnel strips the environment, so NSELF_V15=1 is passed explicitly: released
// cli 1.4.x has `config secrets resolve` only with it (D-1236).
func signerResolver(root string) secrets.ValueResolver {
	return &secrets.Resolver{Runner: &secrets.ExecRunner{Home: os.Getenv("NSELF_CI_HOME"), CoordinatorID: "signer", Attempt: "key"}, Project: root, Env: []string{"NSELF_V15=1"}}
}

// signerStore writes the key with `nself config secrets set` over stdin.
// The key-management lock is per project and lives in the user's home (not in
// the project, and not in the CI home), so two processes with different CI
// homes still serialize and the repository gains no file.
func signerStore(root string) (signing.SecretStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &signing.CLIStore{Runner: &signing.FunnelRunner{Home: os.Getenv("NSELF_CI_HOME")}, Project: root, LockDir: filepath.Join(home, ".nself", "locks")}, nil
}

func signerRequest(ctx context.Context, root string) int {
	req, payloads, err := signing.DecodeRequest(signerStdin())
	if err != nil {
		return signerFail(err)
	}
	st, err := cirun.OpenStore(runsStorePath())
	if err != nil {
		return signerFail(signing.Failf(signing.ReasonSignerFailed, "the run store could not be opened"))
	}
	defer func() { _ = st.Close() }()
	blobs, err := cas.New(records.StoreDir(os.Getenv("NSELF_CI_HOME")))
	if err != nil {
		return signerFail(signing.Failf(signing.ReasonSignerFailed, "the blob store could not be opened"))
	}
	envs, err := signing.Handler{Src: st, Blobs: blobs, Project: root, Resolver: signerResolver(root)}.Handle(ctx, req, payloads)
	if err != nil {
		return signerFail(err)
	}
	return signerWrite(signing.Response{Envelopes: envs})
}

func signerKey(info signing.KeyInfo, err error) int {
	if err != nil {
		return signerFail(err)
	}
	return signerWrite(info)
}

func signerWrite(v any) int {
	if err := json.NewEncoder(signerStdout()).Encode(v); err != nil {
		return 2
	}
	return 0
}

// signerFail prints the refusal as the coordinator reads it back.
func signerFail(err error) int {
	reason := signing.ReasonSignerFailed
	if se, ok := err.(*signing.Error); ok {
		reason = se.Reason
	}
	_ = json.NewEncoder(signerStdout()).Encode(signing.Response{Error: &signing.ResponseError{Code: signing.CodeSigning, Reason: reason, Message: err.Error()}})
	return 2
}
