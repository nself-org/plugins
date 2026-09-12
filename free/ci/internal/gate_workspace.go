package internal

// Package internal — gate_workspace.go
//
// Purpose: Resolve a Node repo's workspace member packages so the node gate
//   can see scripts that live in member packages instead of only the root
//   package.json — declared (pnpm-workspace.yaml / package.json
//   "workspaces") or, failing that, merely nested with no formal declaration
//   at all.
// Inputs:  root string — repo root
// Outputs: nodeWorkspace{kind, members}
// Constraints: A declared workspace always wins (respects the author's exact
//   glob intent, including exclusions); the implicit walk is a fallback
//   only, bounded to implicitMemberMaxDepth and skipping dependency/build/VCS
//   directories, so it cannot balloon into a full-repo crawl.
// SPORT: PLUGINS-CI-007 (G-015)
//
// WHY this exists as a fallback at all: nself-org/plugins has 5 nested
// package.json files (e.g. ".workers/plugins-registry",
// "free/feature-flags/sdk-ts") and declares NEITHER a pnpm-workspace.yaml NOR
// a "workspaces" field. The prior workspaceMembers() understood only those
// two formal declarations, found nothing, and the node gate ran zero checks
// while gitleaks alone reported "Overall: PASSED" — the shape of bug G-015
// closes.

import (
	"os"
	"path/filepath"
	"strings"
)

// workspaceKind distinguishes a formally declared pnpm/npm workspace — which
// supports `pnpm -r` / `npm --workspaces` recursion — from implicitly
// discovered member packages, which must be run one directory at a time
// since no workspace manifest tells the package manager they are related.
type workspaceKind int

const (
	// workspaceNone: no pnpm-workspace.yaml, no "workspaces" field, and the
	// fallback walk found no nested package.json either.
	workspaceNone workspaceKind = iota
	// workspaceDeclared: pnpm-workspace.yaml or package.json "workspaces"
	// names the members explicitly.
	workspaceDeclared
	// workspaceImplicit: nothing declared a workspace, but nested
	// package.json files were found by walking the tree.
	workspaceImplicit
)

// nodeWorkspace is the result of resolving a Node repo's workspace members.
type nodeWorkspace struct {
	kind    workspaceKind
	members []string
}

// implicitMemberMaxDepth bounds the fallback walk in discoverNestedMembers.
// 4 covers every case seen in nself-org repos (e.g. plugins'
// free/<plugin>/ts/package.json sits 3 directories below the repo root)
// without turning into an unbounded crawl of an entire monorepo.
const implicitMemberMaxDepth = 4

// skipNestedDirs are directories never worth descending into while
// discovering implicit workspace members: dependency trees, build output,
// and VCS metadata. Deliberately does NOT skip all dot-directories — plugins'
// own ".workers/plugins-registry" is a real member with real scripts.
var skipNestedDirs = map[string]bool{
	"node_modules": true, ".git": true, "dist": true, "build": true,
	"out": true, "coverage": true, ".next": true, ".turbo": true,
	".cache": true, "vendor": true, "target": true, ".venv": true,
}

// detectNodeWorkspace resolves a Node repo's workspace members. Declared
// patterns (pnpm-workspace.yaml or package.json "workspaces") always take
// priority over the implicit fallback walk.
func detectNodeWorkspace(root string) nodeWorkspace {
	if patterns := declaredWorkspacePatterns(root); len(patterns) > 0 {
		return nodeWorkspace{kind: workspaceDeclared, members: globMembers(root, patterns)}
	}
	if members := discoverNestedMembers(root, implicitMemberMaxDepth); len(members) > 0 {
		return nodeWorkspace{kind: workspaceImplicit, members: members}
	}
	return nodeWorkspace{kind: workspaceNone}
}

// declaredWorkspacePatterns reads pnpm-workspace.yaml globs, or failing that,
// package.json's "workspaces" array. Members outside the repo (sibling
// checkouts referenced via "..") are not ours to gate.
//
// The package.json path was previously dead code: workspaceMembers() read
// loadPackageJSON(root)["workspaces"], but loadPackageJSON only ever returns
// the "scripts" sub-object, so that type assertion could never succeed — a
// "workspaces" field was silently never honoured. This reads the raw file.
func declaredWorkspacePatterns(root string) []string {
	if data, err := os.ReadFile(filepath.Join(root, "pnpm-workspace.yaml")); err == nil {
		var patterns []string
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "- ") {
				continue
			}
			pat := strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "- ")), `'"`)
			if pat != "" && !strings.HasPrefix(pat, "..") {
				patterns = append(patterns, pat)
			}
		}
		return patterns
	}

	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil
	}
	var patterns []string
	for _, pat := range extractJSONStringArray(string(data), "workspaces") {
		if !strings.HasPrefix(pat, "..") {
			patterns = append(patterns, pat)
		}
	}
	return patterns
}

// globMembers expands declared workspace glob patterns to absolute member
// directories that actually contain a package.json.
func globMembers(root string, patterns []string) []string {
	seen := map[string]bool{}
	var members []string
	for _, pat := range patterns {
		matches, err := filepath.Glob(filepath.Join(root, pat))
		if err != nil {
			continue
		}
		for _, m := range matches {
			if strings.Contains(m, "node_modules") || seen[m] {
				continue
			}
			if fileExists(filepath.Join(m, "package.json")) {
				seen[m] = true
				members = append(members, m)
			}
		}
	}
	return members
}

// discoverNestedMembers walks root — bounded to maxDepth, skipping
// dependency/build/VCS directories — collecting every directory that holds
// its own package.json. Used only as a fallback when no pnpm-workspace.yaml
// or "workspaces" field declares members explicitly; see detectNodeWorkspace.
func discoverNestedMembers(root string, maxDepth int) []string {
	var members []string

	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > maxDepth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() || skipNestedDirs[e.Name()] {
				continue
			}
			sub := filepath.Join(dir, e.Name())
			if fileExists(filepath.Join(sub, "package.json")) {
				members = append(members, sub)
			}
			walk(sub, depth+1)
		}
	}
	walk(root, 1)
	return members
}
