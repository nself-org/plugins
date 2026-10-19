package truth

import "testing"

const factsRules = `version: 1
include: [.github/truth/claims.yaml]
overrides:
  claims.current-version:
    facts: .github/facts.json
`

// TestVersionClaim: a semver literal that names the product fails, unless it
// equals the facts version or sits in a generated region.
func TestVersionClaim(t *testing.T) {
	files := map[string]string{
		".github/truth/claims.yaml": realClaims(t),
		".github/truth.yaml":        factsRules,
		".github/facts.json":        "{\"version\": \"1.4.15\", \"counts\": {}}\n",
		"README.md":                 "nself v1.4.15 is out.\nnSelf CLI 1.4.14 is out.\nLatest release: v1.4.15\nCurrent version: `1.4.16`\nRequires Go 1.22.0.\nnself 1.4.15-rc1\n",
		"docs/in.md":                "<!-- BEGIN GENERATED:facts -->\nnself v9.9.9\n<!-- END GENERATED:facts -->\n",
		"CHANGELOG.md":              "nself v1.0.0\n",
	}
	wantFindings(t, findingsOf(t, files, ".github/truth.yaml"),
		"claims.current-version README.md:2", "claims.current-version README.md:4", "claims.current-version README.md:6")
}

// TestVersionClaimLooksLikeAnAddress: an address or a longer dotted number is not a version.
func TestVersionClaimLooksLikeAnAddress(t *testing.T) {
	got := claimFindings(t, map[string]string{
		"a.md": "Listen on nself@127.0.0.1 only.\nnself 1.2.3.4 build\nnself v1.4.15, then\nnself v1.4.15.\nnself v1.4.15-rc.1 works\n",
	})
	wantFindings(t, got, "claims.current-version a.md:3", "claims.current-version a.md:4", "claims.current-version a.md:5")
}

// TestVersionClaimNeedsFacts: with no facts file every literal fails.
func TestVersionClaimNeedsFacts(t *testing.T) {
	got := claimFindings(t, map[string]string{"README.md": "nself v1.4.15 is out.\n"})
	wantFindings(t, got, "claims.current-version README.md:1")
}

// TestVersionClaimRuleLevelFacts: a rule file may carry `facts:` itself.
func TestVersionClaimRuleLevelFacts(t *testing.T) {
	rules := "version: 1\nrules:\n  - {id: v, kind: version-claim, pattern: 'release [0-9.]+', message: m, paths: ['*.md'], facts: facts.yaml}\n"
	got := findingsOf(t, map[string]string{"r.yaml": rules, "facts.yaml": "version: v2.0.1\n", "a.md": "release 2.0.1\nrelease 2.0.2\nrelease two\n"}, "r.yaml")
	wantFindings(t, got, "v a.md:2")
}

func TestVersionFactsErrors(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"missing":    {},
		"no version": {".github/facts.json": "{\"counts\": 1}\n"},
		"not semver": {".github/facts.json": "{\"version\": \"latest\"}\n"},
		"not a map":  {".github/facts.json": "[1, 2]\n"},
	} {
		t.Run(name, func(t *testing.T) {
			files["a.md"] = "x\n"
			files[".github/truth/claims.yaml"] = realClaims(t)
			files[".github/truth.yaml"] = factsRules
			dir := writeTree(t, files)
			rs, err := Load(dir, []string{".github/truth.yaml"})
			if err != nil {
				t.Fatal(err)
			}
			_, err = Scan(dir, rs, rs.Files)
			mustErr(t, err, ".github/truth/claims.yaml", "rules.claims.current-version.facts")
			checks := Run(dir, []string{".github/truth.yaml"}, "")
			if checks[0].Result != "error" || checks[0].FailureClass != "code" {
				t.Errorf("%+v", checks[0])
			}
		})
	}
}
