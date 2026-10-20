//go:build security && unix

package redteam

// Purpose: isolating regression tests added after the mutation review of the HELD claims (P7-TRUST-18).
//   - tar: the lexical name and link-target rules of authz.ArtifactPath, checked below the os.Root layer
//     (directly, and through specs.Validate whose root is nil), with a lone escaping symlink entry that no
//     later write follows;
//   - artifacts get: each ArtifactGetDest rule on a fixture that no other rule refuses.
// Inputs:  authz.ArtifactPath, authz.Extractor, specs.Validate, authz.ArtifactGetDest.
// Outputs: pass (control held) or fail (attack worked).
// Constraints: temp dirs only; every refusal is checked by its reason, not only by "an error".

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
)

func pathReason(err error) authz.PathReason {
	var pe *authz.PathError
	if errors.As(err, &pe) {
		return pe.Reason
	}
	return ""
}

// The lexical rules, one entry at a time: no filesystem root is involved, so os.Root cannot mask a rule.
func TestB_ArtifactPath_LexicalRulesStandAlone(t *testing.T) {
	file := func(name string) authz.Entry { return authz.Entry{Name: name, Mode: 0o644} }
	link := func(name, to string) authz.Entry {
		return authz.Entry{Name: name, LinkTarget: to, Mode: fs.ModeSymlink | 0o777}
	}
	for _, ok := range []authz.Entry{file("a/b"), file("a/./b"), link("l", "sub"), link("d/l", "../x")} {
		if _, err := authz.ArtifactPath(ok, authz.Limits{}); err != nil {
			t.Errorf("control: %+v must be admitted: %v", ok, err)
		}
	}
	for name, c := range map[string]struct {
		e    authz.Entry
		want authz.PathReason
	}{
		"parent":             {file("../x"), authz.PathTraversal},
		"parent in the root": {file("a/../b"), authz.PathTraversal},
		"absolute":           {file("/x"), authz.PathAbsolute},
		"lone link out":      {link("l", "../../x"), authz.LinkEscape},
		"link out of depth":  {link("d/l", "../../x"), authz.LinkEscape},
		"link absolute":      {link("l", "/tmp"), authz.LinkAbsolute},
		"link climb late":    {link("d/l", "sub/../../x"), authz.LinkEscape},
	} {
		_, err := authz.ArtifactPath(c.e, authz.Limits{})
		if got := pathReason(err); got != c.want {
			t.Errorf("%s: reason %q (%v), want %q", name, got, err, c.want)
		}
	}
}

// The same lone entries through the Extractor and through specs.Validate (nil root, dry run).
func TestB_ArtifactPath_LoneEntriesThroughExtractorAndValidate(t *testing.T) {
	if err := specs.Validate(bytes.NewReader(buildTar(t, []tarEnt{dir("d"), sym("d/l", "../x"), reg("f", "x")}, false)), authz.Limits{}); err != nil {
		t.Fatalf("control: an in-root link must validate: %v", err)
	}
	for name, ents := range map[string][]tarEnt{
		"parent":        {reg("../x", "x")},
		"parent inside": {reg("a/../b", "x")},
		"absolute":      {reg("/x", "x")},
		"lone link out": {sym("l", "../../x")},
		"lone link abs": {sym("l", "/tmp")},
	} {
		if err := specs.Validate(bytes.NewReader(buildTar(t, ents, false)), authz.Limits{}); err == nil {
			t.Errorf("%s: Validate accepted a hostile entry with no root to catch it", name)
		}
		x := authz.NewExtractor(authz.Limits{})
		for _, te := range ents {
			e := authz.Entry{Name: te.h.Name, LinkTarget: te.h.Linkname, Mode: 0o644}
			if te.h.Linkname != "" {
				e.Mode = fs.ModeSymlink | 0o777
			}
			if _, err := x.Admit(e); err == nil {
				t.Errorf("%s: Extractor.Admit accepted %q", name, te.h.Name)
			}
		}
	}
}

// Artifact get destinations: one fixture per rule, none of them also refused by another rule.
func TestB_ArtifactGetDest_EachRuleOnItsOwn(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mk := func(p string) string {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	empty := mk(filepath.Join(tmp, "cwd"))
	plain := mk(filepath.Join(tmp, "plain"))
	if got, err := authz.ArtifactGetDest("fresh", empty); err != nil || got != filepath.Join(empty, "fresh") {
		t.Fatalf("control: a new name under an empty cwd must be accepted: %q %v", got, err)
	}
	if got, err := authz.ArtifactGetDest(plain, empty); err != nil || got != plain {
		t.Fatalf("control: an empty plain directory must be accepted: %q %v", got, err)
	}
	// dest is the (empty) cwd itself
	for _, dest := range []string{".", empty} {
		if _, err := authz.ArtifactGetDest(dest, empty); pathReason(err) != authz.DestCwd {
			t.Errorf("dest %q equal to an empty cwd: reason %q (%v), want %q", dest, pathReason(err), err, authz.DestCwd)
		}
	}
	// a symlink leaf that points at a plain empty directory
	leaf := filepath.Join(tmp, "leaf")
	if err := os.Symlink(plain, leaf); err != nil {
		t.Fatal(err)
	}
	if _, err := authz.ArtifactGetDest(leaf, empty); pathReason(err) != authz.DestSymlink {
		t.Errorf("symlink leaf to a plain directory: reason %q (%v), want %q", pathReason(err), err, authz.DestSymlink)
	}
	// the spelled name has a .git component but resolves to a plain directory
	gitLink := filepath.Join(mk(filepath.Join(tmp, "g1")), ".git")
	if err := os.Symlink(plain, gitLink); err != nil {
		t.Fatal(err)
	}
	if _, err := authz.ArtifactGetDest(filepath.Join(gitLink, "out"), empty); pathReason(err) != authz.DestUnderGit {
		t.Errorf("a .git component in the spelled dest: reason %q (%v), want %q", pathReason(err), err, authz.DestUnderGit)
	}
	// the spelled name is clean but resolves under a .git directory
	realGit := mk(filepath.Join(tmp, "g2", ".git"))
	cleanLink := filepath.Join(tmp, "g2link")
	if err := os.Symlink(realGit, cleanLink); err != nil {
		t.Fatal(err)
	}
	if _, err := authz.ArtifactGetDest(filepath.Join(cleanLink, "out"), empty); pathReason(err) != authz.DestUnderGit {
		t.Errorf("a dest that resolves under .git: reason %q (%v), want %q", pathReason(err), err, authz.DestUnderGit)
	}
}
