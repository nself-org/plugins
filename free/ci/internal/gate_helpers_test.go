// Regression tests for Node workspace member discovery.
//
// WHY these exist: nself-org/plugins declares neither a pnpm-workspace.yaml
// nor a package.json "workspaces" field, yet has 5 nested package.json files
// with real lint/typecheck/test/build scripts. The old workspaceMembers()
// only understood the two formal declarations, found nothing, and the node
// gate silently ran zero checks while gitleaks alone reported "PASSED"
// (G-015). These tests pin the fallback discovery that fixes it, plus the
// package.json "workspaces" field path, which the old code referenced via
// loadPackageJSON(root)["workspaces"] — a type assertion that could never
// succeed, because loadPackageJSON only ever returns the "scripts" object.
package internal

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writePackageJSON(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write package.json in %s: %v", dir, err)
	}
}

// TestDetectNodeWorkspace_ImplicitDiscoveryMatchesPluginsShape reproduces the
// exact nself-org/plugins layout: a root package.json with only an unrelated
// script ("ci:local"), no pnpm-workspace.yaml, no "workspaces" field, and
// nested package.json files at varying depths (mirrors ".workers/plugins-registry"
// and "free/feature-flags/sdk-ts"). Before the fix this produced workspaceKind
// == workspaceNone and zero members; it must now find both.
func TestDetectNodeWorkspace_ImplicitDiscoveryMatchesPluginsShape(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, root, `{"version":"1.0.0","scripts":{"ci:local":"echo ok"}}`)
	writePackageJSON(t, filepath.Join(root, ".workers", "plugins-registry"),
		`{"name":"registry","scripts":{"typecheck":"tsc --noEmit","test":"node --test"}}`)
	writePackageJSON(t, filepath.Join(root, "free", "feature-flags", "sdk-ts"),
		`{"name":"sdk-ts","scripts":{"build":"tsc","test":"jest"}}`)

	ws := detectNodeWorkspace(root)

	if ws.kind != workspaceImplicit {
		t.Fatalf("expected workspaceImplicit (no formal declaration, nested package.json present), got kind=%d members=%v", ws.kind, ws.members)
	}
	if len(ws.members) != 2 {
		t.Fatalf("expected 2 implicit members, got %d: %v", len(ws.members), ws.members)
	}
	wantA := filepath.Join(root, ".workers", "plugins-registry")
	wantB := filepath.Join(root, "free", "feature-flags", "sdk-ts")
	if !slices.Contains(ws.members, wantA) {
		t.Errorf("expected member %s, got %v", wantA, ws.members)
	}
	if !slices.Contains(ws.members, wantB) {
		t.Errorf("expected member %s, got %v", wantB, ws.members)
	}
}

// TestDiscoverNestedMembers_SkipsDependencyAndBuildDirs asserts the fallback
// walk never descends into node_modules/dist/.git — a repo where every real
// package.json lives inside node_modules (typical after `pnpm install`) must
// not report thousands of vendored packages as workspace members.
func TestDiscoverNestedMembers_SkipsDependencyAndBuildDirs(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, filepath.Join(root, "node_modules", "some-dep"), `{"name":"some-dep"}`)
	writePackageJSON(t, filepath.Join(root, "dist"), `{"name":"build-output"}`)
	writePackageJSON(t, filepath.Join(root, "real-app"), `{"name":"real-app","scripts":{"test":"true"}}`)

	members := discoverNestedMembers(root, implicitMemberMaxDepth)

	if len(members) != 1 {
		t.Fatalf("expected exactly 1 real member (node_modules/dist must be skipped), got %d: %v", len(members), members)
	}
	if members[0] != filepath.Join(root, "real-app") {
		t.Errorf("expected real-app, got %v", members)
	}
}

// TestDiscoverNestedMembers_RespectsMaxDepth ensures the walk does not surface
// a package.json buried deeper than the configured bound, keeping the
// fallback a scoped discovery rather than a full-repo crawl.
func TestDiscoverNestedMembers_RespectsMaxDepth(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c", "d", "e", "f")
	writePackageJSON(t, deep, `{"name":"too-deep"}`)

	members := discoverNestedMembers(root, 2)

	if len(members) != 0 {
		t.Fatalf("expected 0 members beyond maxDepth, got %v", members)
	}
}

// TestDetectNodeWorkspace_DeclaredPnpmWorkspaceWins asserts a real
// pnpm-workspace.yaml still takes priority over the implicit walk and is
// resolved via its own glob semantics, unchanged from before this fix.
func TestDetectNodeWorkspace_DeclaredPnpmWorkspaceWins(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, root, `{"name":"root"}`)
	if err := os.WriteFile(filepath.Join(root, "pnpm-workspace.yaml"), []byte("packages:\n  - 'packages/*'\n"), 0o644); err != nil {
		t.Fatalf("write pnpm-workspace.yaml: %v", err)
	}
	writePackageJSON(t, filepath.Join(root, "packages", "a"), `{"name":"a","scripts":{"test":"true"}}`)

	ws := detectNodeWorkspace(root)

	if ws.kind != workspaceDeclared {
		t.Fatalf("expected workspaceDeclared, got kind=%d", ws.kind)
	}
	if len(ws.members) != 1 || ws.members[0] != filepath.Join(root, "packages", "a") {
		t.Fatalf("expected [packages/a], got %v", ws.members)
	}
}

// TestDeclaredWorkspacePatterns_PackageJSONWorkspacesField pins the fix for a
// previously-dead code path: the old workspaceMembers() read
// loadPackageJSON(root)["workspaces"], but loadPackageJSON only ever returns
// the "scripts" sub-object, so that type assertion could never succeed —
// package.json "workspaces" was silently never honoured. This must now work.
func TestDeclaredWorkspacePatterns_PackageJSONWorkspacesField(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, root, `{"name":"root","workspaces":["apps/*","libs/*"]}`)

	patterns := declaredWorkspacePatterns(root)

	if !slices.Contains(patterns, "apps/*") || !slices.Contains(patterns, "libs/*") {
		t.Fatalf("expected [\"apps/*\" \"libs/*\"], got %v", patterns)
	}
}

// TestDetectNodeWorkspace_NoPackageAnywhereIsNone asserts a plain single
// package (no nested package.json, no declaration) still resolves to
// workspaceNone rather than spuriously discovering itself.
func TestDetectNodeWorkspace_NoPackageAnywhereIsNone(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, root, `{"name":"solo","scripts":{"test":"true"}}`)

	ws := detectNodeWorkspace(root)

	if ws.kind != workspaceNone {
		t.Fatalf("expected workspaceNone for a repo with no nested members, got kind=%d members=%v", ws.kind, ws.members)
	}
}

// TestSkipReasonForScript_NamesTheScriptAndMemberCount asserts a skip reason
// is never a bare, unexplained line — it must name the script and say
// whether a workspace was even considered.
func TestSkipReasonForScript_NamesTheScriptAndMemberCount(t *testing.T) {
	none := skipReasonForScript("lint", nodeWorkspace{kind: workspaceNone})
	if !strings.Contains(none, `"lint"`) || !strings.Contains(none, "no workspace members found") {
		t.Errorf("workspaceNone reason missing detail: %q", none)
	}

	withMembers := skipReasonForScript("build", nodeWorkspace{kind: workspaceImplicit, members: []string{"a", "b", "c"}})
	if !strings.Contains(withMembers, `"build"`) || !strings.Contains(withMembers, "3 workspace member") {
		t.Errorf("workspaceImplicit reason missing member count: %q", withMembers)
	}
}
