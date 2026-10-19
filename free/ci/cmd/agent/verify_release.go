package main

// Purpose: `nself-ci-agent verify-release --pub FILE [--version V] DIR`: the operator-side check of a built release directory
//          (hq NODE D11, P7-NODE-15). It verifies every nself-ci-agent-<os>-<arch> file in DIR against its
//          .manifest.json, .cdx.json, .spdx.json and .intoto.dsse.json (P7-CACHE-10) with the keys of one PEM file, through the same
//          internal/agent/verify package `nodes add` uses.
// Inputs:  --pub FILE (PEM SPKI Ed25519 public key, exactly one block; a rehearsal-test.pub holds two: the manifest key and the
//          provenance key; there is no key discovery), --version V (default: DIR/VERSION, else the
//          version the manifests agree on), --revoked FILE (revoked key ids; a manifest signed by one fails), --require-all-targets (the release workflow sets it: all of linux-amd64, linux-arm64
//          and darwin-arm64 must be present), DIR from build.sh and manifest.sh.
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
	revokedFile := fs.String("revoked", "", "file of revoked key ids, one agent-<16 hex> per line (scripts/release/keys/agent-revoked.txt)")
	tagsOnly := fs.Bool("tags-only", false, "accept only provenance built from the tag refs/tags/v<version> (the release workflow sets it; a rehearsal runs from a branch)")
	all := fs.Bool("require-all-targets", false, "fail unless linux-amd64, linux-arm64 and darwin-arm64 are all present")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *pubPath == "" || fs.NArg() != 1 {
		fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent verify-release --pub FILE [--version V] [--tags-only] DIR")
		return 2
	}
	v, err := verifierFromPEM(*pubPath, *revokedFile)
	if err != nil {
		fmt.Fprintf(sio.Stderr, "verify-release: %v\n", err)
		return 1
	}
	if *tagsOnly {
		v.RequireTagBuilds()
	}
	return verifyReleaseDir(v, fs.Arg(0), *version, *all, sio.Stdout, sio.Stderr)
}

// releaseTargets are the targets every real release carries (hq NODE D11).
var releaseTargets = []string{"linux-amd64", "linux-arm64", "darwin-arm64"}

// verifierFromPEM builds a Verifier trusting exactly the keys in the PEM file.
func verifierFromPEM(path, revokedPath string) (*verify.Verifier, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read the public key file")
	}
	var keys []signing.Key
	rest := data
	for {
		var block *pem.Block
		if block, rest = pem.Decode(rest); block == nil {
			break
		}
		if block.Type != "PUBLIC KEY" {
			return nil, fmt.Errorf("the public key file holds a PEM block that is not a PUBLIC KEY")
		}
		k, err := x509.ParsePKIXPublicKey(block.Bytes)
		pub, ok := k.(ed25519.PublicKey)
		if err != nil || !ok {
			return nil, fmt.Errorf("the public key is not Ed25519")
		}
		keys = append(keys, signing.Key{ID: signing.KeyID(signing.PurposeAgent, pub), Purpose: signing.PurposeAgent, Public: pub})
	}
	// A committed agent key file holds exactly one key (genkeys.sh compiles only that one into clients, so a second key here would pass
	// the gate and then be refused everywhere). Only a rehearsal directory's rehearsal-test.pub holds the manifest key and the provenance
	// key.
	most := 1
	if filepath.Base(path) == "rehearsal-test.pub" {
		most = 4
	}
	if len(keys) == 0 || len(keys) > most || strings.TrimSpace(string(rest)) != "" {
		return nil, fmt.Errorf("the public key file must hold exactly one PEM PUBLIC KEY block (only rehearsal-test.pub may hold up to four)")
	}
	var revoked []string
	if revokedPath != "" {
		rf, oerr := os.Open(revokedPath)
		if oerr != nil {
			return nil, fmt.Errorf("cannot read the revoked file")
		}
		defer func() { _ = rf.Close() }()
		if revoked, err = signing.ParseRevokedFile(rf); err != nil {
			return nil, fmt.Errorf("the revoked file is not valid: %v", err)
		}
	}
	return verify.New(keys, revoked)
}

func verifyReleaseDir(v *verify.Verifier, dir, version string, requireAll bool, stdout, stderr io.Writer) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintln(stderr, "verify-release: cannot read the release directory")
		return 1
	}
	bins, manifests, sides := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		switch {
		case !strings.HasPrefix(name, "nself-ci-agent-"):
		case isSideFile(name):
			sides[name] = true
		case strings.HasSuffix(name, ".manifest.json"):
			manifests[strings.TrimSuffix(name, ".manifest.json")] = true
		default:
			bins[name] = true
		}
	}
	// A side file with neither a binary nor a manifest would be uploaded by the release workflow without ever being checked
	// (one with a manifest fails below as "manifest without a binary").
	for name := range sides {
		if base := sideBase(name); !bins[base] && !manifests[base] {
			fmt.Fprintf(stderr, "%s: %s: %s: %s has no binary in the directory\n", name, verify.CodeVerify, verify.ReasonProvenance, name)
			return 1
		}
	}
	if len(bins) == 0 {
		fmt.Fprintln(stderr, "verify-release: no nself-ci-agent-<os>-<arch> file in the directory")
		return 1
	}
	if requireAll {
		for _, t := range releaseTargets {
			if !bins["nself-ci-agent-"+t] {
				fmt.Fprintf(stderr, "verify-release: %s: target %s is missing from the release\n", verify.CodeVerify, t)
				return 1
			}
		}
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
	mpath := filepath.Join(dir, name+".manifest.json")
	if lst, lerr := os.Lstat(mpath); lerr != nil || !lst.Mode().IsRegular() {
		return fmt.Errorf("%s: %s: the manifest is not a regular file", verify.CodeVerify, verify.ReasonUnreadable)
	}
	raw, err := os.ReadFile(mpath)
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
	files, err := readSideFiles(dir, name)
	if err != nil {
		return err
	}
	m, err := v.Artifact(filepath.Join(dir, name), raw, verify.Expect{Version: *version, Target: strings.TrimPrefix(name, "nself-ci-agent-")}, files)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "ok %s %s %s\n", name, m.Version, m.KeyID)
	return nil
}

// sideSuffixes are the files that sit next to a binary besides its manifest (P7-CACHE-10).
var sideSuffixes = []string{verify.SuffixCycloneDX, verify.SuffixSPDX, verify.SuffixProvenance}

// isSideFile reports whether name is an SBOM or provenance file rather than a binary.
func isSideFile(name string) bool {
	for _, suf := range sideSuffixes {
		if strings.HasSuffix(name, suf) {
			return true
		}
	}
	return false
}

// sideBase is the binary name a side file belongs to.
func sideBase(name string) string {
	for _, suf := range sideSuffixes {
		if strings.HasSuffix(name, suf) {
			return strings.TrimSuffix(name, suf)
		}
	}
	return name
}

// readSideFiles reads the two SBOMs and the provenance of one binary; a missing, special or oversized file is the provenance reason.
func readSideFiles(dir, name string) (verify.Files, error) {
	var files verify.Files
	for _, f := range []struct {
		suffix string
		into   *[]byte
		limit  int64
	}{{verify.SuffixProvenance, &files.Provenance, verify.MaxProvenanceBytes}, {verify.SuffixCycloneDX, &files.CycloneDX, verify.MaxSBOMBytes}, {verify.SuffixSPDX, &files.SPDX, verify.MaxSBOMBytes}} {
		b, err := readRegular(filepath.Join(dir, name+f.suffix), f.limit)
		if err != nil {
			return verify.Files{}, fmt.Errorf("%s: %s: %s%s: %v", verify.CodeVerify, verify.ReasonProvenance, name, f.suffix, err)
		}
		*f.into = b
	}
	return files, nil
}

// readRegular reads a regular file (never a symlink) of at most limit bytes.
func readRegular(path string, limit int64) ([]byte, error) {
	lst, err := os.Lstat(path)
	if err != nil || !lst.Mode().IsRegular() {
		return nil, fmt.Errorf("missing or not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot be opened")
	}
	defer func() { _ = f.Close() }()
	if st, serr := f.Stat(); serr != nil || !os.SameFile(lst, st) {
		return nil, fmt.Errorf("changed while it was opened")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, fmt.Errorf("unreadable or larger than %d bytes", limit)
	}
	return b, nil
}
