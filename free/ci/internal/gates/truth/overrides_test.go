package truth

import "testing"

const orgRules = `version: 1
rules:
  - {id: retired-oldname, kind: retired-term, term: Old Name, message: say New Name}
`

func TestOverridesAddExemptPaths(t *testing.T) {
	repo := "version: 1\ninclude: [org.yaml]\noverrides:\n  retired-oldname:\n    exempt_paths:\n      - {glob: 'history/**', reason: the history folder keeps old names}\n"
	files := map[string]string{"org.yaml": orgRules, "truth.yaml": repo, "history/a.md": "Old Name\n", "docs/b.md": "Old Name\n"}
	wantFindings(t, findingsOf(t, files, "truth.yaml"), "retired-oldname docs/b.md:1")
}

func TestOverrideErrors(t *testing.T) {
	for name, tc := range map[string]struct{ body, key string }{
		"unknown rule":  {"overrides:\n  nope:\n    exempt_paths: [{glob: 'a/**', reason: a long enough reason}]\n", "overrides.nope"},
		"no reason":     {"overrides:\n  retired-oldname:\n    exempt_paths: [{glob: 'a/**', reason: short}]\n", "overrides.retired-oldname.exempt_paths[0].reason"},
		"bad glob":      {"overrides:\n  retired-oldname:\n    exempt_paths: [{glob: '/a', reason: a long enough reason}]\n", "overrides.retired-oldname.exempt_paths[0].glob"},
		"facts on term": {"overrides:\n  retired-oldname:\n    facts: f.json\n", "overrides.retired-oldname.facts"},
		"unknown key":   {"overrides:\n  retired-oldname:\n    allow: []\n", "allow"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{"org.yaml": orgRules, "truth.yaml": "version: 1\ninclude: [org.yaml]\n" + tc.body})
			_, err := Load(dir, []string{"truth.yaml"})
			mustErr(t, err, "truth.yaml", tc.key)
		})
	}
}

// TestExemptMatches: a hit inside an exempt_matches span passes; the same
// literal outside it fails; a dead-host rule takes none.
func TestExemptMatches(t *testing.T) {
	got := claimFindings(t, map[string]string{
		"a.md": "```go\nimport \"github.com/" + oldSlug() + "/paid/ai\"\n```\n",
		"b.md": "See github.com/" + oldSlug() + " for more\n",
		"c.md": "import \"github.com/" + oldSlug() + "/paid/ai\" and github.com/" + oldSlug() + "\n",
		"d.md": "Plain " + oldSlug() + "\n",
	})
	wantFindings(t, got, "vocab.bundles-repo-slug b.md:1", "vocab.bundles-repo-slug c.md:1", "vocab.bundles-repo-slug d.md:1")
}

func TestExemptMatchErrors(t *testing.T) {
	h := HashLiteral(plantedHost())
	for name, tc := range map[string]struct{ body, key string }{
		"dead host":   {"{id: a, kind: dead-host, sha256: [" + h + "], message: m, exempt_matches: [{regex: x, reason: a long enough reason}]}", "rules[0].exempt_matches"},
		"no reason":   {"{id: a, kind: banned-literal, literal: x, message: m, exempt_matches: [{regex: x, reason: no}]}", "rules[0].exempt_matches[0].reason"},
		"bad regex":   {"{id: a, kind: banned-literal, literal: x, message: m, exempt_matches: [{regex: '(', reason: a long enough reason}]}", "rules[0].exempt_matches[0].regex"},
		"path no why": {"{id: a, kind: banned-literal, literal: x, message: m, exempt_paths: [{glob: 'a/**', reason: x}]}", "rules[0].exempt_paths[0].reason"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{"r.yaml": "version: 1\nrules:\n  - " + tc.body + "\n"})
			_, err := Load(dir, []string{"r.yaml"})
			mustErr(t, err, "r.yaml", tc.key)
		})
	}
}

// TestVocabulary: "Pro plugin(s)" fails in any case and plural; look-alikes pass.
func TestVocabulary(t *testing.T) {
	got := claimFindings(t, map[string]string{
		"a.md":         "The pro plugin and " + proPlugins() + " and PRO PLUGINS.\nA Product plugin, a pro pluginx, improve plugins.\n",
		"CHANGELOG.md": proPlugins() + "\n",
	})
	wantFindings(t, got, "vocab.pro-plugin a.md:1")
}

// TestExemptMatchesAcrossLines: a hit that spans a line break honours
// exempt_matches exactly as the same hit on one line does.
func TestExemptMatchesAcrossLines(t *testing.T) {
	rules := "version: 1\nrules:\n" +
		"  - {id: c, kind: numeric-claim, pattern: '62\\s+plugins', message: m, paths: ['*.md'], exempt_matches: [{regex: '62\\s+plugins', reason: a long enough reason}]}\n" +
		"  - {id: d, kind: numeric-claim, pattern: '63\\s+plugins', message: m, paths: ['*.md']}\n"
	got := findingsOf(t, map[string]string{"r.yaml": rules, "a.md": "62 plugins\n", "b.md": "62\nplugins\n", "c.md": "63\nplugins\n"}, "r.yaml")
	wantFindings(t, got, "d c.md:1")
}
