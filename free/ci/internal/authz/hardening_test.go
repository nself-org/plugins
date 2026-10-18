package authz_test

import (
	"io/fs"
	"math"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

// Round-2 review findings: normalization aliases, .git planting, overflow.

func admitAll(es []authz.Entry, lim authz.Limits) error {
	x := authz.NewExtractor(lim)
	for _, e := range es {
		if _, err := x.Admit(e); err != nil {
			return err
		}
	}
	return nil
}

func file(name string) authz.Entry { return authz.Entry{Name: name, Mode: 0o644, Size: 1, Packed: 1} }
func dir(name string) authz.Entry  { return authz.Entry{Name: name, Mode: fs.ModeDir | 0o755} }
func link(name, to string) authz.Entry {
	return authz.Entry{Name: name, LinkTarget: to, Mode: fs.ModeSymlink | 0o777}
}

// The APFS alias chain: NFC and NFD spellings of one directory.
func TestExtractor_UnicodeAliasWithSymlinkRefused(t *testing.T) {
	nfc, nfd := "d/\u00e9", "d/e\u0301"
	chain := []authz.Entry{dir("d/"), link(nfc, ".."), link(nfd+"/l2", "../.."), file("l2/pwn")}
	if err := admitAll(chain, authz.Limits{}); err == nil {
		t.Fatal("alias chain admitted")
	}
	// order independence: the non-ASCII name first, the symlink after
	if err := admitAll([]authz.Entry{file("\u00e9.txt"), link("l", "x")}, authz.Limits{}); err == nil {
		t.Fatal("symlink after non-ASCII name admitted")
	}
	if err := admitAll([]authz.Entry{link("l", "\u00e9")}, authz.Limits{}); err == nil {
		t.Fatal("non-ASCII link target admitted")
	}
	// non-ASCII names alone, and symlinks alone, stay legal
	if err := admitAll([]authz.Entry{dir("d/"), file("d/\u00e9.txt")}, authz.Limits{}); err != nil {
		t.Fatalf("plain unicode refused: %v", err)
	}
	if err := admitAll([]authz.Entry{dir("d/"), link("d/l", "../x")}, authz.Limits{}); err != nil {
		t.Fatalf("plain symlink refused: %v", err)
	}
}

func TestArtifactPath_DotGit(t *testing.T) {
	for _, n := range []string{".git/config", "a/.GIT/hooks/x", ".git", "a/.git./x", "a/.git /x", "GIT~1/config", "x/.Git"} {
		if _, err := authz.ArtifactPath(authz.Entry{Name: n, Mode: 0o644, Size: 1, Packed: 1}, authz.Limits{}); err == nil {
			t.Errorf("%q admitted", n)
		}
	}
	for _, n := range []string{".github/x", ".gitignore", "git/x", "a.git"} {
		if _, err := authz.ArtifactPath(authz.Entry{Name: n, Mode: 0o644, Size: 1, Packed: 1}, authz.Limits{}); err != nil {
			t.Errorf("%q refused: %v", n, err)
		}
	}
	if _, err := authz.ArtifactPath(authz.Entry{Name: ".git/config", Mode: 0o644, Size: 1, Packed: 1}, authz.Limits{AllowDotGit: true}); err != nil {
		t.Errorf("opt-in refused: %v", err)
	}
}

func TestArtifactPath_FormatRunesRefused(t *testing.T) {
	for _, n := range []string{".g\u200cit/config", "a\u202eb", "\xef\xbb\xbfa"} {
		if _, err := authz.ArtifactPath(file(n), authz.Limits{}); err == nil {
			t.Errorf("%q admitted", n)
		}
	}
}

// Format runes real names use (Persian ZWNJ, emoji ZWJ) stay legal.
func TestArtifactPath_LegitFormatRunesAdmitted(t *testing.T) {
	for _, n := range []string{"\u0645\u06cc\u200c\u062e\u0648\u0627\u0647\u0645.txt", "family\U0001F468\u200d\U0001F469.png"} {
		if _, err := authz.ArtifactPath(file(n), authz.Limits{}); err != nil {
			t.Errorf("%q refused: %v", n, err)
		}
	}
}

func TestExtractor_TotalSizeDoesNotWrap(t *testing.T) {
	lim := authz.Limits{MaxFileBytes: math.MaxInt64, MaxTotalBytes: math.MaxInt64, RatioFloor: math.MaxInt64}
	big := authz.Entry{Mode: 0o644, Size: math.MaxInt64 - 1, Packed: 1}
	x := authz.NewExtractor(lim)
	big.Name = "a"
	if _, err := x.Admit(big); err != nil {
		t.Fatalf("first: %v", err)
	}
	big.Name = "b"
	if _, err := x.Admit(big); err == nil {
		t.Fatal("second entry wrapped the total")
	}
}

func TestArtifactPath_LinkToDotGitRefused(t *testing.T) {
	if _, err := authz.ArtifactPath(link("l", ".git/hooks"), authz.Limits{}); err == nil {
		t.Fatal("link into .git admitted")
	}
	if _, err := authz.ArtifactPath(link("l", ".git/hooks"), authz.Limits{AllowDotGit: true}); err != nil {
		t.Fatalf("opt-in refused: %v", err)
	}
}

func TestArtifactPath_DriveAfterDotComponent(t *testing.T) {
	for _, n := range []string{"./C:/x", "././Z:", "./0:/x"} {
		if _, err := authz.ArtifactPath(file(n), authz.Limits{}); err == nil {
			t.Errorf("%q admitted", n)
		}
	}
}

func TestArtifactPath_RatioIsExact(t *testing.T) {
	over := authz.Entry{Name: "a", Mode: 0o644, Size: 1_050_001, Packed: 10_500}
	if _, err := authz.ArtifactPath(over, authz.Limits{}); err == nil {
		t.Fatal("ratio 100.00001 admitted")
	}
	at := authz.Entry{Name: "a", Mode: 0o644, Size: 1_050_000, Packed: 10_500}
	if _, err := authz.ArtifactPath(at, authz.Limits{}); err != nil {
		t.Fatalf("ratio exactly 100 refused: %v", err)
	}
}
