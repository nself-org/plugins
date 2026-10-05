package main

// Purpose: the "build" subcommand: local signed release-artifact build lane
// (P6-E11-W2-S1-T6). Private repos need a signed artifact without a
// GitHub-hosted runner or a third nSelf server. Android only.
// Inputs:  argv after "build": --artifact (default android, as core),
// --upload, --tag, --owner, --repo, -v/--verbose, --timeout, [dir].
// Outputs: build log to stdout, a signed APK, optional gh release upload;
// exit 0/1.
// Constraints: the argv the 1.4.x proxy sends (--artifact X --timeout N [-v]
// [--upload --tag T [--owner O] [--repo R]] DIR) keeps its exact meaning
// (EPIC D11). Core's fail-fast "--tag is required with --upload" check runs
// before the build, as in core.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal"
)

func init() {
	register("build", func(args []string) int { return runArtifactBuildCmd(args) })
}

// buildOpts is the parsed "build" request.
type buildOpts struct {
	artifact string
	upload   bool
	tag      string
	owner    string
	repo     string
	verbose  bool
	timeout  int
	dir      string
}

// parseBuildArgs parses the build argv; flags and the directory may be mixed.
func parseBuildArgs(rawArgs []string, eh flag.ErrorHandling) (buildOpts, error) {
	var o buildOpts
	fs := flag.NewFlagSet("build", eh)
	fs.StringVar(&o.artifact, "artifact", "android", "Artifact type to build (only \"android\" is supported)")
	fs.BoolVar(&o.upload, "upload", false, "Attach the built artifact to a GitHub release via `gh release upload`")
	fs.StringVar(&o.tag, "tag", "", "Release tag to upload to (required with --upload)")
	fs.StringVar(&o.owner, "owner", "", "GitHub owner for --upload (default: from git remote)")
	fs.StringVar(&o.repo, "repo", "", "GitHub repo for --upload (default: from git remote)")
	fs.BoolVar(&o.verbose, "v", false, "Print each command before running")
	fs.BoolVar(&o.verbose, "verbose", false, "Alias for -v (core spelling)")
	fs.IntVar(&o.timeout, "timeout", 900, "Build step timeout in seconds (release builds are slow)")
	pos, err := parseInterspersed(fs, rawArgs)
	if err != nil {
		return o, err
	}
	o.dir = "."
	if len(pos) > 0 {
		o.dir = pos[0]
	}
	return o, nil
}

// runArtifactBuildCmd implements "nself-ci build --artifact android [dir]".
func runArtifactBuildCmd(rawArgs []string) int {
	o, err := parseBuildArgs(rawArgs, flag.ExitOnError)
	if err != nil {
		return 2
	}

	if o.upload && o.tag == "" {
		fmt.Fprintln(os.Stderr, "Error: --tag is required with --upload")
		return 1
	}
	if o.artifact != "android" {
		fmt.Fprintf(os.Stderr, "error: --artifact %q not supported (only \"android\" — macOS/Windows/TV/WearOS stay on GitHub-hosted runners)\n", o.artifact)
		return 1
	}

	androidDir := o.dir
	// Core made the directory absolute before running the binary.
	if abs, aerr := filepath.Abs(o.dir); aerr == nil {
		androidDir = abs
	}

	fmt.Printf("nself-ci build --artifact android — %s\n", androidDir)
	fmt.Println(strings.Repeat("─", 60))

	result := internal.BuildAndroidArtifact(androidDir, o.timeout, o.verbose)
	mark := "PASS"
	if !result.Gate.Passed {
		mark = "FAIL"
	}
	fmt.Printf("  %-30s  %s  (%s)\n", result.Gate.Name, mark, result.Gate.Elapsed.Round(time.Millisecond))
	if result.Gate.Output != "" {
		for _, line := range strings.SplitAfter(result.Gate.Output, "\n") {
			fmt.Print("    ", line)
		}
		fmt.Println()
	}
	fmt.Println(strings.Repeat("─", 60))

	if !result.Gate.Passed {
		return 1
	}
	if !o.upload {
		return 0
	}
	return uploadBuilt(o, androidDir, result.APKPath)
}

// uploadBuilt attaches the APK to the release named by o.tag.
func uploadBuilt(o buildOpts, androidDir, apk string) int {
	resolvedOwner, resolvedRepo := o.owner, o.repo
	if resolvedOwner == "" || resolvedRepo == "" {
		ro, rr, err := internal.RepoOwnerName(androidDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot resolve GitHub remote for --upload: %v\n", err)
			fmt.Fprintf(os.Stderr, "hint: pass --owner and --repo\n")
			return 1
		}
		if resolvedOwner == "" {
			resolvedOwner = ro
		}
		if resolvedRepo == "" {
			resolvedRepo = rr
		}
	}
	out, err := internal.UploadArtifact(resolvedOwner, resolvedRepo, o.tag, apk)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Printf("✓ Uploaded %s to %s/%s@%s\n%s\n", apk, resolvedOwner, resolvedRepo, o.tag, out)
	return 0
}
