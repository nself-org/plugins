package truth

// Purpose: D-1261. The shipped truth rules must also catch the spelling variants of a retired
// name or a typed count: ASCII nTV, the hyphenated or double-spaced Pro plugin and Plugins Pro,
// language-qualified plugin counts and a bare count in a heading.
// Constraints: the retired names are built at run time (this file is scanned by the org rules).

import (
	"os"
	"path/filepath"
	"testing"
)

// readRule is a shipped rule file of the repository.
func readRule(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", ".github", "truth", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func asciiNTV() string { return "n" + "TV" }

func TestRetiredTermSeparatorRuns(t *testing.T) {
	low := "buy the pro  plugins, pro plugin, pro\tplugin and pro plugin"
	if got := len(termSpans(low, "pro plugin(s)")); got != 4 {
		t.Errorf("a space in a term must match any run of white space: got %d spans, want 4", got)
	}
	if got := len(termSpans("proplugin", "pro plugin")); got != 0 {
		t.Errorf("a space in a term must not match no space: got %d spans", got)
	}
}

func TestASCIIRetiredProduct(t *testing.T) {
	body := "The " + asciiNTV() + " app is live.\nthe NTV bundle\nSee ntv for more.\nnself-org/ntv is archived.\nA ntvx and an intv.\n"
	got := claimFindings(t, map[string]string{"a.md": body, "CHANGELOG.md": "Added " + asciiNTV() + "\n"})
	wantFindings(t, got, "vocab.ntv-ascii a.md:1", "vocab.ntv-ascii a.md:2", "vocab.ntv-ascii a.md:3")
}

func TestProPluginSpellingVariants(t *testing.T) {
	body := "A Pro-plugin here.\nTwo Pro  plugins here.\nThe pro plugin here.\nSee [docs](/docs/pro-plugins) now.\nA Product plugin.\nPro plugins, plain.\n"
	got := claimFindings(t, map[string]string{"a.md": body})
	wantFindings(t, got, "vocab.pro-plugin a.md:2", "vocab.pro-plugin a.md:3", "vocab.pro-plugin a.md:6", "vocab.pro-plugin-hyphen a.md:1")
}

func TestPluginsProSpellingVariants(t *testing.T) {
	body := "Buy Plugins-Pro today.\nBuy Plugins  Pro today.\nSee github.com/nself-org/plugins-pro/ here.\nThe `plugins-pro` folder, the plugins-pro repo.\nOpen plugins-pro/paid/x or ../Plugins-Pro/x.\n"
	got := claimFindings(t, map[string]string{"a.md": body})
	// Plugins Pro with a space or two is an org.yaml term (all files); the claims rule owns the hyphen.
	wantFindings(t, got, "vocab.plugins-pro-hyphen a.md:1", "vocab.bundles-repo-slug a.md:3")
}

func TestPluginsProSpacedVariantsInOrgRules(t *testing.T) {
	files := map[string]string{"a.md": "Buy Plugins Pro today.\nBuy Plugins  Pro today.\nBuy Plugins Pro today.\n"}
	files[".github/truth/org.yaml"] = readRule(t, "org.yaml")
	got := findingsOf(t, files, ".github/truth/org.yaml")
	wantFindings(t, got, "retired-plugins-pro a.md:1", "retired-plugins-pro a.md:2", "retired-plugins-pro a.md:3")
}

func TestLanguageQualifiedAndHeadingCounts(t *testing.T) {
	body := "We ship 58 Go plugins.\nThere are 12 TypeScript plugins.\n### Family (12)\n## Install (1)\n### Brand — baseline (7)\n### Health Checks (4)\nFamily (12) in a sentence.\nWe ship 58 plugins.\nGo plugins are small.\n"
	got := claimFindings(t, map[string]string{"a.md": body})
	wantFindings(t, got, "claims.counts a.md:1", "claims.counts a.md:2", "claims.counts a.md:8",
		"claims.counts-heading a.md:3", "claims.counts-heading a.md:4", "claims.counts-heading a.md:5")
}
