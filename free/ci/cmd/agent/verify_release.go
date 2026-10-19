package main

// Purpose: `nself-ci-agent verify-release --pub FILE [--version V] DIR`: the operator-side check of a built release directory
//          (hq NODE D11, P7-NODE-15). It verifies every nself-ci-agent-<os>-<arch> file in DIR against its
//          .manifest.json with exactly one public key, through the same internal/agent/verify package `nodes add` uses.
// Inputs:  --pub FILE (PEM SPKI Ed25519 public key; there is no key discovery), --version V (default: DIR/VERSION, else the
//          version the manifests agree on), DIR from build.sh and manifest.sh.
// Outputs: one "ok <file> <version> <key id>" line per file on stdout; failures "E665: <reason>: <detail>" on stderr.
//          Exit 0 only when every binary and manifest pair verifies, 1 on a failure, 2 on usage.
// Constraints: a binary with no manifest, a manifest with no binary or an empty directory never passes. Nothing in DIR is
//          executed or modified. The check refuses a key that is not an Ed25519 public key.

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
)

func init() {
	must(RegisterSubcommand("verify-release", "verify a built agent release directory against one public key", runVerifyRelease))
}

func runVerifyRelease(_ context.Context, args []string, sio IO) int {
	fs := flag.NewFlagSet("verify-release", flag.ContinueOnError)
	fs.SetOutput(sio.Stderr)
	pubPath := fs.String("pub", "", "PEM SPKI Ed25519 public key to verify against")
	version := fs.String("version", "", "expected version (default: DIR/VERSION)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *pubPath == "" || fs.NArg() != 1 {
		fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent verify-release --pub FILE [--version V] DIR")
		return 2
	}
	v, err := verifierFromPEM(*pubPath)
	if err != nil {
		fmt.Fprintf(sio.Stderr, "verify-release: %v\n", err)
		return 1
	}
	return verifyReleaseDir(v, fs.Arg(0), *version, sio.Stdout, sio.Stderr)
}

// verifierFromPEM builds a Verifier trusting exactly the key in the PEM file.
func verifierFromPEM(path string) (*verify.Verifier, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read the public key file")
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("the public key file is not a PEM PUBLIC KEY")
	}
	k, err := x509.ParsePKIXPublicKey(block.Bytes)
	pub, ok := k.(ed25519.PublicKey)
	if err != nil || !ok {
		return nil, fmt.Errorf("the public key is not Ed25519")
	}
	return verify.New([]signing.Key{{ID: signing.KeyID(signing.PurposeAgent, pub), Purpose: signing.PurposeAgent, Public: pub}}, nil)
}

func verifyReleaseDir(v *verify.Verifier, dir, version string, stdout, stderr io.Writer) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintln(stderr, "verify-release: cannot read the release directory")
		return 1
	}
	bins, manifests := map[string]bool{}, map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		switch {
		case !strings.HasPrefix(name, "nself-ci-agent-"):
		case strings.HasSuffix(name, ".manifest.json"):
			manifests[strings.TrimSuffix(name, ".manifest.json")] = true
		default:
			bins[name] = true
		}
	}
	if len(bins) == 0 {
		fmt.Fprintln(stderr, "verify-release: no nself-ci-agent-<os>-<arch> file in the directory")
		return 1
	}
	if version == "" {
		if b, rerr := os.ReadFile(filepath.Join(dir, "VERSION")); rerr == nil {
			version = strings.TrimSpace(string(b))
		}
	}
	names := make([]string, 0, len(bins)+len(manifests))
	for n := range bins {
		names = append(names, n)
	}
	for n := range manifests {
		if !bins[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	failed := false
	for _, n := range names {
		if err := verifyOne(v, dir, n, bins[n], manifests[n], &version, stdout); err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", n, err)
			failed = true
		}
	}
	if failed {
		return 1
	}
	return 0
}

// verifyOne checks one binary/manifest pair. version is filled from the first manifest when no version was given, and every
// later manifest must agree with it.
func verifyOne(v *verify.Verifier, dir, name string, hasBin, hasManifest bool, version *string, stdout io.Writer) error {
	if !hasBin {
		return fmt.Errorf("%s: manifest without a binary", verify.CodeVerify)
	}
	if !hasManifest {
		return fmt.Errorf("%s: %s: no manifest for this binary", verify.CodeVerify, verify.ReasonMissingSignature)
	}
	raw, err := os.ReadFile(filepath.Join(dir, name+".manifest.json"))
	if err != nil {
		return fmt.Errorf("%s: %s: cannot read the manifest", verify.CodeVerify, verify.ReasonUnreadable)
	}
	if *version == "" {
		m, perr := verify.ParseManifest(raw)
		if perr != nil {
			return perr
		}
		*version = m.Version
	}
	m, err := v.Artifact(filepath.Join(dir, name), raw, verify.Expect{Version: *version, Target: strings.TrimPrefix(name, "nself-ci-agent-")})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "ok %s %s %s\n", name, m.Version, m.KeyID)
	return nil
}
