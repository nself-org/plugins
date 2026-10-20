package main

// Purpose: `providers github init --check` (Epic D5): compare the workflow on the default branch and
// on the untrusted ref, as local git has them, with the rendering, and print the exact commands that
// create or update the untrusted ref.
// Inputs:  the repository root, the rendering, providers.github.untrusted_ref.
// Outputs: one line per copy (ok, missing, drifted), the commands to fix what is not ok, exit 1 naming
//          every ref that drifted or is missing.
// Constraints: reads local git only (git show origin/<ref>:<path>); never fetches, never pushes: the
//          commands are printed for the operator; a failure to read git is reported as missing.

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/report"
	githubtmpl "github.com/nself-org/plugins/free/ci/templates/github"
)

type githubCopy struct {
	Ref    string `json:"ref"`
	Status string `json:"status"`
}

func gitShow(ctx context.Context, root, ref string) (string, bool) {
	out, exit, err := report.Capture(ctx, "git", "-c", "core.fsmonitor=false", "-C", root, "show", ref+":"+githubtmpl.Path)
	return out, err == nil && exit == 0
}

func defaultRemoteBranch(ctx context.Context, root string) string {
	out, exit, err := report.Capture(ctx, "git", "-c", "core.fsmonitor=false", "-C", root, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if b := strings.TrimPrefix(strings.TrimSpace(out), "origin/"); err == nil && exit == 0 && b != "" {
		return b
	}
	return "main"
}

func githubCheck(ctx context.Context, f githubInitFlags, root string, doc []byte, untrusted string) int {
	def := defaultRemoteBranch(ctx, root)
	var copies []githubCopy
	for _, branch := range []string{def, untrusted} {
		status := "ok"
		switch got, ok := gitShow(ctx, root, "origin/"+branch); {
		case !ok:
			status = "missing"
		case got != string(doc):
			status = "drifted"
		}
		copies = append(copies, githubCopy{Ref: "origin/" + branch, Status: status})
	}
	bad := false
	for _, c := range copies {
		bad = bad || c.Status != "ok"
	}
	if f.json {
		_ = output.WriteData(os.Stdout, "ci providers github init", map[string]any{"copies": copies, "ok": !bad}, nil)
	} else {
		for _, c := range copies {
			fmt.Printf("%-32s %s\n", c.Ref, c.Status)
		}
		githubFixes(copies, def, untrusted)
	}
	if bad {
		var names []string
		for _, c := range copies {
			if c.Status != "ok" {
				names = append(names, c.Ref)
			}
		}
		fmt.Fprintf(os.Stderr, "error: %s %s the rendering\n", strings.Join(names, ", "), map[bool]string{true: "differ from", false: "differs from"}[len(names) > 1])
		return 1
	}
	return 0
}

// githubFixes prints what the operator runs to bring the copies in line (nothing here runs them).
func githubFixes(copies []githubCopy, def, untrusted string) {
	if copies[0].Status != "ok" {
		fmt.Printf("\nthe default branch copy is %s: run nself ci providers github init, commit %s and push it to %s\n", copies[0].Status, githubtmpl.Path, def)
		return
	}
	switch copies[1].Status {
	case "missing":
		fmt.Printf("\ncreate the untrusted ref from the default branch:\n  git fetch origin\n  git push origin origin/%s:refs/heads/%s\n", def, untrusted)
	case "drifted":
		fmt.Printf("\nupdate the untrusted ref to the default branch:\n  git fetch origin\n  git push --force-with-lease origin origin/%s:refs/heads/%s\n", def, untrusted)
	}
}
