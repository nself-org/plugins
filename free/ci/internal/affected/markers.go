package affected

// Purpose: the two kinds of directory the graph cannot reason about. An
// opaque directory holds a manifest of an ecosystem this package has no graph
// for, when the tree holds more than one manifest of that ecosystem (crates,
// Dart packages, Python projects can depend on each other by path). A
// generator directory holds a code generator config, whose inputs and
// outputs are not imports.
// Inputs:  the walked tree.
// Outputs: Graph.opaque and Graph.generators; a change inside either makes
// the selector return a full run.
// Constraints: pure; testdata/ directories are ignored.

import (
	"path"
	"sort"
	"strings"
)

var opaqueManifests = map[string]string{
	"Cargo.toml": "cargo", "pubspec.yaml": "pub", "pyproject.toml": "python", "setup.py": "python",
	"Gemfile": "ruby", "pom.xml": "maven", "build.gradle": "gradle", "build.gradle.kts": "gradle",
	"Package.swift": "swift", "composer.json": "php", "mix.exs": "mix", "CMakeLists.txt": "cmake",
	"BUILD": "bazel", "BUILD.bazel": "bazel", "WORKSPACE": "bazel", "MODULE.bazel": "bazel",
	"build.sbt": "sbt", "deno.json": "deno", "deno.jsonc": "deno",
}

// opaqueExts are manifest extensions of ecosystems with project references.
var opaqueExts = map[string]string{".csproj": "dotnet", ".fsproj": "dotnet", ".vbproj": "dotnet", ".cabal": "cabal"}

var generatorConfigs = map[string]bool{
	"sqlc.yaml": true, "sqlc.yml": true, "sqlc.json": true, "buf.yaml": true, "buf.gen.yaml": true,
	"gqlgen.yml": true, "gqlgen.yaml": true, ".gqlgen.yml": true, "codegen.yml": true, "codegen.yaml": true,
	"codegen.ts": true, "codegen.json": true, "buf.work.yaml": true,
}

func markers(t tree, g *Graph) {
	dirs := map[string]map[string]bool{} // ecosystem -> manifest directories
	for _, f := range t.files {
		d := dirOf(f)
		if !notTestdata(d) {
			continue
		}
		base := path.Base(f)
		eco, ok := opaqueManifests[base]
		if !ok {
			eco, ok = opaqueExts[path.Ext(base)]
		}
		if ok {
			if dirs[eco] == nil {
				dirs[eco] = map[string]bool{}
			}
			dirs[eco][d] = true
		}
		if generatorConfigs[base] || isAPISpec(base) {
			g.generators[d] = true
		}
	}
	ecos := make([]string, 0, len(dirs))
	for eco := range dirs {
		ecos = append(ecos, eco)
	}
	sort.Strings(ecos)
	for _, eco := range ecos {
		if len(dirs[eco]) < 2 {
			continue
		}
		for d := range dirs[eco] {
			if _, taken := g.opaque[d]; !taken {
				g.opaque[d] = eco
			}
		}
	}
}

func isAPISpec(base string) bool {
	if !strings.HasPrefix(base, "openapi") && !strings.HasPrefix(base, "swagger") {
		return false
	}
	switch path.Ext(base) {
	case ".yaml", ".yml", ".json":
		return true
	}
	return false
}
