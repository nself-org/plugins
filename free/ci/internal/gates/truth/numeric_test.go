package truth

import "testing"

// TestNumericClaimFixtures: hand counts fail with the right rule id; text that
// only looks like a count passes.
func TestNumericClaimFixtures(t *testing.T) {
	got := claimFindings(t, map[string]string{
		"README.md":       "We ship 129 free plugins.\nOver 50+ plugins and 20 commands, 12 verbs, 3 bundles, 7 integrations.\n",
		"docs/known.md":   "Install one plugin with `nself add`.\nThe 20-verb canon.\nSee §5 Plugin and HTTP 404 plugin-not-found.\nADR 0016 verbs.\nRelease 1.2.3 plugins ship.\nPlan 3.2 commands later.\n",
		"docs/rest.txt":   "129 plugins\n",
		"src/main.go":     "// 129 plugins\n",
		"go.mod":          "module x // 129 plugins\n",
		"settings.json":   "{\"title\": \"129 plugins\"}\n",
		"CHANGELOG.md":    "129 plugins\n",
		"a/testdata/b.md": "129 plugins\n",
	})
	wantFindings(t, got,
		"claims.counts README.md:1", "claims.counts README.md:2", "claims.counts docs/rest.txt:1")
}

// TestNumericClaimArabic: Western and Arabic-Indic digits before the noun fail
// claims.counts-ar in i18n values; the same words without a count pass.
func TestNumericClaimArabic(t *testing.T) {
	got := claimFindings(t, map[string]string{
		"org/messages/ar.json": "{\n \"a\": \"CLI + 32 إضافة مجانية\",\n \"b\": \"٣٢ إضافات\",\n \"c\": \"إضافة جديدة\",\n \"d\": \"۱۲ أمر\"\n}\n",
		"org/config/ar.json":   "{\"a\": \"32 إضافة\"}\n",
	})
	wantFindings(t, got, "claims.counts-ar org/messages/ar.json:2", "claims.counts-ar org/messages/ar.json:3", "claims.counts-ar org/messages/ar.json:5")
}

// TestNumericClaimMatchText: the finding carries the matched text.
func TestNumericClaimMatchText(t *testing.T) {
	dir := writeTree(t, map[string]string{".github/truth/claims.yaml": realClaims(t), "README.md": "(129 free plugins)\n"})
	rs, err := Load(dir, []string{".github/truth/claims.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := Scan(dir, rs, rs.Files)
	if err != nil || len(res.Findings) != 1 || res.Findings[0].Match != "129 free plugins" {
		t.Fatalf("%v %+v", err, res.Findings)
	}
}

func TestNumericClaimRuleFileErrors(t *testing.T) {
	for name, tc := range map[string]struct{ body, key string }{
		"no pattern":     {"{id: a, kind: numeric-claim, message: m, paths: ['*.md']}", "rules[0].pattern"},
		"bad pattern":    {"{id: a, kind: numeric-claim, pattern: '(', message: m, paths: ['*.md']}", "rules[0].pattern"},
		"no paths":       {"{id: a, kind: numeric-claim, pattern: x, message: m}", "rules[0].paths"},
		"literal on it":  {"{id: a, kind: version-claim, pattern: x, literal: y, message: m, paths: ['*.md']}", "rules[0].pattern"},
		"pattern on lit": {"{id: a, kind: banned-literal, literal: x, pattern: y, message: m}", "rules[0].pattern"},
		"facts on count": {"{id: a, kind: numeric-claim, pattern: x, facts: f.json, message: m, paths: ['*.md']}", "rules[0].facts"},
		"facts escapes":  {"{id: a, kind: version-claim, pattern: x, facts: ../f.json, message: m, paths: ['*.md']}", "rules[0].facts"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{"r.yaml": "version: 1\nrules:\n  - " + tc.body + "\n"})
			_, err := Load(dir, []string{"r.yaml"})
			mustErr(t, err, "r.yaml", tc.key)
		})
	}
}
