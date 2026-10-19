package truth

import (
	"strings"
	"testing"
)

// TestOverrideNeedsAnInclude: an override applies only to rules in files the declaring file includes.
func TestOverrideNeedsAnInclude(t *testing.T) {
	dead := "version: 1\nrules:\n  - {id: dead.old, kind: dead-host, sha256: [" + HashLiteral(plantedHost()) + "], message: gone}\n"
	child := "version: 1\noverrides:\n  dead.old:\n    exempt_paths: [{glob: '**', reason: a long enough reason}]\n"
	top := "version: 1\ninclude: [dead.yaml, child.yaml]\n"
	dir := writeTree(t, map[string]string{"dead.yaml": dead, "child.yaml": child, "top.yaml": top, "a.md": plantedHost() + "\n"})
	_, err := Load(dir, []string{"top.yaml"})
	mustErr(t, err, "child.yaml", "overrides.dead.old")
	// the same override in a file that includes the rule is fine
	ok := "version: 1\ninclude: [dead.yaml]\n" + strings.TrimPrefix(child, "version: 1\n")
	dir = writeTree(t, map[string]string{"dead.yaml": dead, "ok.yaml": ok})
	if _, err := Load(dir, []string{"ok.yaml"}); err != nil {
		t.Fatal(err)
	}
}

// TestAstroComponentsAreNotCodeBlocks: <StyledCard> and <ScriptureQuote /> open no script or style block.
func TestAstroComponentsAreNotCodeBlocks(t *testing.T) {
	c := "We ship 129 free plugins."
	body := "---\n---\n<StyledCard>\n<p>" + c + "</p>\n</StyledCard>\n<ScriptureQuote />\n<p>" + c + "</p>\n<style>\n/* 129 plugins */\n</style>\n"
	got := claimFindings(t, map[string]string{"a.astro": body})
	wantFindings(t, got, "claims.counts a.astro:4", "claims.counts a.astro:7")
}

// TestCountVariants: emphasis around the number, other spaces and the brand name do not hide a count.
func TestCountVariants(t *testing.T) {
	lines := []string{"- **62** plugins across it", "We ship 87&nbsp;plugins", "87\u202fplugins", "87\u2009plugins", "87\u200b plugins", "We ship 87 nSelf plugins", "**87 plugins**",
		"Run nself plugin install", "10 nself plugin install"}
	var body string
	for _, l := range lines {
		body += l + "\n"
	}
	got := claimFindings(t, map[string]string{"a.md": body})
	wantFindings(t, got, "claims.counts a.md:1", "claims.counts a.md:2", "claims.counts a.md:3", "claims.counts a.md:4", "claims.counts a.md:5", "claims.counts a.md:6", "claims.counts a.md:7")
}

func TestVersionVariants(t *testing.T) {
	body := "**nSelf** v1.2.0\n`nself` 1.2.0\nnSelf (v1.2.0)\nnSelf\u00a0v1.2.0\nnSelf version 1.2.0\nnself-cli 1.2.0\nnself-ci v1.2.0\n"
	var want []string
	for i := 1; i <= 6; i++ {
		want = append(want, "claims.current-version a.md:"+string(rune('0'+i)))
	}
	wantFindings(t, claimFindings(t, map[string]string{"a.md": body}), want...)
}

// TestStringLiteralExpressionsAreNotExempt: a JSX string that prints typed text is still typed text.
func TestStringLiteralExpressionsAreNotExempt(t *testing.T) {
	got := claimFindings(t, map[string]string{"a.mdx": "{\"We ship 87 plugins today\"}\n{`We ship 88 plugins ${x}`}\n{count + ' plugins'}\n{`87 plugins`}\n"})
	wantFindings(t, got, "claims.counts a.mdx:1", "claims.counts a.mdx:4")
}

// TestMarkersInProseAndMDX: prose that mentions the marker exempts nothing; MDX uses JSX comments.
func TestMarkersInProseAndMDX(t *testing.T) {
	got := claimFindings(t, map[string]string{
		"a.md":  "Write `<!-- truth: exempt your reason here -->` above the line.\n" + count,
		"b.md":  "```\n<!-- truth: exempt a reason inside a fence -->\n```\n" + count,
		"c.mdx": "{/* truth: exempt quoted from the launch post */}\n" + count,
		"d.mdx": "{/* BEGIN GENERATED:x */}\n" + count + "{/* END GENERATED:x */}\n",
		"e.mdx": "{/* truth: exempt */}\n" + count,
	})
	wantFindings(t, got, "claims.counts a.md:2", "claims.counts b.md:4", "truth.exempt-without-reason e.mdx:1", "claims.counts e.mdx:2")
}

// TestDeadHostNeverPrinted: a dead host in a file name or in another rule's match is redacted.
func TestDeadHostNeverPrinted(t *testing.T) {
	h := HashLiteral(plantedHost())
	rules := "version: 1\nrules:\n  - {id: dead, kind: dead-host, sha256: [" + h + "], message: gone}\n  - {id: local.servers, kind: numeric-claim, pattern: 'server [0-9.]+', message: m, paths: ['*.md']}\n"
	dir := writeTree(t, map[string]string{"r.yaml": rules, "docs/a.md": "server " + plantedHost() + "\n", "docs/" + plantedHost() + ".md": "server 9 plugins\n"})
	code, out, _ := runMain("", "check", "--root", dir, "r.yaml")
	if code != 1 || strings.Contains(out, plantedHost()) {
		t.Fatalf("%d\n%s", code, out)
	}
	_, js, _ := runMain("", "check", "--root", dir, "--json", "r.yaml")
	if strings.Contains(js, plantedHost()) {
		t.Errorf("json leaks: %s", js)
	}
}

// TestControlCharactersInPaths: a file name cannot forge report lines.
func TestControlCharactersInPaths(t *testing.T) {
	dir := writeTree(t, map[string]string{"docs/a.md\nPASS truth.x (0)\nz.md": count, ".github/truth/claims.yaml": realClaims(t)})
	_, out, _ := runMain("", "check", "--root", dir, ".github/truth/claims.yaml")
	if strings.Contains(out, "\nPASS truth.x") {
		t.Errorf("forged line:\n%s", out)
	}
}

// TestLongLineIsLinear: one very long line of exempt hits finishes quickly.
func TestLongLineIsLinear(t *testing.T) {
	line := strings.Repeat("github.com/"+oldSlug()+"/sdk/ ", 40000) + "\n"
	got := claimFindings(t, map[string]string{"a.md": line})
	wantFindings(t, got)
}

// TestDeltaReviewFixes: self-closing script tags, Markdown headings, dashes and release folders.
func TestDeltaReviewFixes(t *testing.T) {
	c := "We ship 89 plugins."
	got := claimFindings(t, map[string]string{
		"sc.astro":                "---\n---\n<script src=\"a.js\" />\n<p>" + c + "</p>\n<Script>\n<p>" + c + "</p>\n",
		"gh.md":                   "# Why files are GENERATED BY tools\n\n" + c + "\n",
		"gh2.md":                  "<!-- GENERATED BY tools/facts DO NOT HAND EDIT -->\n\n" + c + "\n",
		"steps.md":                "## Step 3 - Commands\nPhase 2 - Integrations\na 62-plugin catalog\n",
		"docs/releases/v1.2.0.md": c + "\n",
		"docs/releases/now.md":    "nSelf v1.2.0 is the current release\n",
	})
	wantFindings(t, got, "claims.counts sc.astro:4", "claims.counts sc.astro:6", "claims.counts gh.md:3",
		"claims.counts steps.md:3", "claims.current-version docs/releases/now.md:1")
}
