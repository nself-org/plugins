package main

// Purpose: `go run ./tools/provgen`, the thin main over internal/provenance.BuildAgent that scripts/release/agent/provenance.sh calls
//          (hq CACHE D17, P7-CACHE-10). It writes the in-toto Statement v1 (SLSA provenance v1, buildType agent/v1) of ONE agent
//          binary and the DSSE pre-authentication encoding of that statement, which the script signs with the agent key.
// Inputs:  --binary PATH (the built nself-ci-agent file; its base name is the subject name), --sha256 HEX (the digest the signed
//          manifest names; must equal the file's), --sbom PATH (repeatable; the base name is the byproduct name),
//          --builder-id WORKFLOW_REF (github.workflow_ref, "<owner>/<repo>/.github/workflows/release-agent.yml@<ref>", with or
//          without a leading https://github.com/), --revision (the commit), --run-id (digits), --out DIR (must not exist).
// Outputs: DIR/statement.json (canonical statement bytes, the DSSE payload) and DIR/statement.pae (signing.PAE of the payload under
//          the in-toto payload type); one "ok <name> <sha256>" line. Exit 0 ok, 1 refused, 2 usage.
// Constraints: pure over its inputs: same inputs, same bytes (no clock, no network, no key). It never signs and never sees a key.

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/plugins/free/ci/internal/provenance"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// builderRe splits a workflow ref into the repository and the git ref the workflow ran at.
var builderRe = regexp.MustCompile(`^(?:https://github\.com/)?([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)/` + regexp.QuoteMeta(provenance.AgentWorkflow) + `@(.+)$`)

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("provgen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	binary := fs.String("binary", "", "built nself-ci-agent file")
	want := fs.String("sha256", "", "sha256 of the binary, as the signed manifest names it")
	builder := fs.String("builder-id", "", "workflow ref: <owner>/<repo>/"+provenance.AgentWorkflow+"@<ref>")
	revision := fs.String("revision", "", "commit the tag points at")
	runID := fs.String("run-id", "", "workflow run id")
	out := fs.String("out", "", "output directory (created, must not exist)")
	var sboms listFlag
	fs.Var(&sboms, "sbom", "SBOM file (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *binary == "" || *want == "" || *builder == "" || *revision == "" || *runID == "" || *out == "" || len(sboms) == 0 || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: provgen --binary PATH --sha256 HEX --sbom PATH [--sbom PATH] --builder-id WORKFLOW_REF --revision SHA --run-id N --out DIR")
		return 2
	}
	if err := generate(*binary, *want, sboms, *builder, *revision, *runID, *out, stdout); err != nil {
		fmt.Fprintf(stderr, "provgen: %v\n", err)
		return 1
	}
	return 0
}

func generate(binary, want string, sboms []string, builder, revision, runID, out string, stdout io.Writer) error {
	m := builderRe.FindStringSubmatch(builder)
	if m == nil {
		return fmt.Errorf("--builder-id is not a %s workflow ref", provenance.AgentWorkflow)
	}
	sum, err := fileDigest(binary)
	if err != nil {
		return err
	}
	if sum != want {
		return fmt.Errorf("the binary's sha256 %s is not the expected %s", sum, want)
	}
	facts := provenance.AgentFacts{Repo: m[1], Ref: m[2], Revision: revision, RunID: runID,
		Subjects: []provenance.FileDigest{{Name: filepath.Base(binary), SHA256: sum}}}
	for _, p := range sboms {
		d, derr := fileDigest(p)
		if derr != nil {
			return derr
		}
		facts.SBOMs = append(facts.SBOMs, provenance.FileDigest{Name: filepath.Base(p), SHA256: d})
	}
	statement, err := provenance.BuildAgent(facts)
	if err != nil {
		return err
	}
	if err = os.Mkdir(out, 0o755); err != nil {
		return fmt.Errorf("cannot create the output directory (it must not exist): %w", err)
	}
	if err = os.WriteFile(filepath.Join(out, "statement.json"), statement, 0o644); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(out, "statement.pae"), signing.PAE(provenance.PayloadType, statement), 0o644); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "ok %s %s\n", facts.Subjects[0].Name, sum)
	return err
}

// fileDigest is the sha256 of a regular file (a symlink is refused: the digest must be of the bytes the release ships).
func fileDigest(path string) (string, error) {
	if lst, err := os.Lstat(path); err != nil || !lst.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", filepath.Base(path))
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot open %s", filepath.Base(path))
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", fmt.Errorf("cannot read %s", filepath.Base(path))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
