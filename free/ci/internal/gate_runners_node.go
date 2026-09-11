package internal

// Package internal — gate_runners_node.go
//
// Purpose: Run lint/typecheck/test/build for a Node repo, across the root
//   package and any workspace members (declared or implicitly discovered —
//   see gate_workspace.go), and explain in the output why any of those four
//   checks never ran anywhere.
// Inputs:  root string, timeout int, verbose bool
// Outputs: []GateResult — one per script that ran, plus one Skipped entry
//   per script that ran nowhere, each carrying the reason in Output.
// Constraints: falls back to npm if pnpm is not on PATH. G-015 —
//   nself-org/plugins has 5 nested package.json files and NEITHER a
//   pnpm-workspace.yaml NOR a "workspaces" field, so the prior member
//   detection found nothing and the gate ran only gitleaks while still
//   reporting "Overall: PASSED".
// SPORT: PLUGINS-CI-007

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// runNodeGates runs lint/typecheck/test/build for a Node repo.
func runNodeGates(root string, timeout int, verbose bool) []GateResult {
	pm := "pnpm"
	if _, err := exec.LookPath("pnpm"); err != nil {
		pm = "npm"
	}

	pkg := loadPackageJSON(root)
	ws := detectNodeWorkspace(root)

	var gates []GateResult
	ranAny := false
	for _, script := range []string{"lint", "typecheck", "test", "build"} {
		scriptGates := runNodeScript(pm, root, script, pkg, ws, timeout, verbose)
		for _, g := range scriptGates {
			if !g.Skipped {
				ranAny = true
			}
		}
		gates = append(gates, scriptGates...)
	}

	// Nothing declared any of the four scripts anywhere, and there is no
	// workspace to recurse into — last resort, run tsc directly if the repo
	// at least declares a tsconfig.json.
	if !ranAny && ws.kind == workspaceNone && fileExists(filepath.Join(root, "tsconfig.json")) {
		gates = append(gates, runStep("node:tsc", root, timeout, verbose, pm, "exec", "tsc", "--noEmit"))
	}

	markSubstantive(gates)
	return gates
}

// runNodeScript runs one script name (lint/typecheck/test/build) against the
// root package and, per ws.kind, its workspace members — returning a Skipped
// gate explaining why when the script exists nowhere.
func runNodeScript(pm, root, script string, pkg map[string]interface{}, ws nodeWorkspace, timeout int, verbose bool) []GateResult {
	var gates []GateResult

	if hasScript(pkg, script) {
		gates = append(gates, runStep("node:"+script, root, timeout, verbose, pm, "run", script))
	}

	switch ws.kind {
	case workspaceDeclared:
		// A real pnpm/npm workspace understands `-r`/`--workspaces` recursion.
		// --if-present so members without the script are skipped rather than
		// failing the whole recursive run.
		if anyMemberHasScript(ws.members, script) {
			gates = append(gates, runStep("node:"+script+" (workspace)", root, timeout, verbose,
				pm, "-r", "--if-present", "run", script))
		}
	case workspaceImplicit:
		// No formal workspace declaration exists, so `pnpm -r` would not
		// recurse into these directories at all — run each member on its own.
		for _, member := range ws.members {
			if !hasScript(loadPackageJSON(member), script) {
				continue
			}
			rel, err := filepath.Rel(root, member)
			if err != nil {
				rel = member
			}
			gates = append(gates, runStep(fmt.Sprintf("node:%s (%s)", script, rel), member, timeout, verbose, pm, "run", script))
		}
	}

	if len(gates) == 0 {
		gates = append(gates, GateResult{
			Name:    "node:" + script,
			Passed:  true,
			Skipped: true,
			Output:  skipReasonForScript(script, ws),
		})
	}

	return gates
}

// skipReasonForScript explains why a given script never ran anywhere, so a
// SKIP entry is never a bare, unexplained line. G-015 / SPORT: PLUGINS-CI-007
func skipReasonForScript(script string, ws nodeWorkspace) string {
	if ws.kind == workspaceNone {
		return fmt.Sprintf("skipped: no %q script in package.json and no workspace members found", script)
	}
	return fmt.Sprintf("skipped: no %q script in package.json or any of %d workspace member(s)", script, len(ws.members))
}
