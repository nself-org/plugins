package affected

// Purpose: the paths whose change always means a full run (Epic D15): the CI
// config, lockfiles, go.mod/go.sum, generator inputs, build scripts and
// workspace definitions. A change here can alter every job, so no graph
// answer is trusted. Root-level tool configuration (tsconfig, linters,
// turbo, nx, bundler configs) is not listed: a root file that no package
// owns is unmapped, which is full too (select.go).
// Inputs:  a repo-relative slash path.
// Outputs: whether it forces a full run.
// Constraints: pure; the list errs on the side of full.

import (
	"path"
	"strings"
)

var fullBases = map[string]bool{
	"go.mod": true, "go.sum": true, "go.work": true, "go.work.sum": true,
	"package-lock.json": true, "npm-shrinkwrap.json": true, "pnpm-lock.yaml": true, "yarn.lock": true,
	"bun.lock": true, "bun.lockb": true, "Cargo.lock": true, "pubspec.lock": true,
	"pnpm-workspace.yaml": true, ".npmrc": true, ".nvmrc": true, ".tool-versions": true, ".pnpmfile.cjs": true,
	"Makefile": true, "makefile": true, "GNUmakefile": true, "Dockerfile": true,
	"sqlc.yaml": true, "sqlc.yml": true, "sqlc.json": true, "buf.yaml": true, "buf.gen.yaml": true,
	"gqlgen.yml": true, "gqlgen.yaml": true, ".gqlgen.yml": true, "codegen.yml": true, "codegen.yaml": true,
	"codegen.ts": true, "codegen.json": true, "buf.work.yaml": true,
}

var fullExts = map[string]bool{
	".mk": true, ".dockerfile": true, ".proto": true, ".graphql": true, ".gql": true, ".tmpl": true, ".gotmpl": true,
}

// ForcesFull reports whether a change to p forces a full run.
func ForcesFull(p string) bool {
	p = path.Clean(strings.TrimPrefix(p, "./"))
	base := path.Base(p)
	switch {
	case p == ".." || strings.HasPrefix(p, "../") || strings.HasPrefix(p, "/"):
		return true // outside the repository: unmapped
	case strings.HasPrefix(p, ".nself/"), strings.HasPrefix(p, ".github/"):
		return true
	case fullBases[base], fullExts[strings.ToLower(path.Ext(base))], strings.HasPrefix(base, "Dockerfile."):
		return true
	case base == "package.json" && path.Dir(p) == ".":
		return true // the workspace root manifest defines the workspace
	case hasSegment(p, "vendor"), hasSegment(p, "node_modules"):
		return true // vendored code is not in the graph and any importer may use it
	case strings.HasSuffix(base, ".sh") && hasSegment(p, "scripts"):
		return true
	case strings.HasPrefix(base, "openapi") || strings.HasPrefix(base, "swagger"):
		return true
	}
	return false
}

func hasSegment(p, seg string) bool {
	for _, s := range strings.Split(path.Dir(p), "/") {
		if s == seg {
			return true
		}
	}
	return false
}
