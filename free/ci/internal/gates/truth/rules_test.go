package truth

import (
	"strings"
	"testing"
)

func TestLoadValidRulesAndInclude(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"rules/main.yaml": "version: 1\ninclude: [rules/more.yaml]\nskip: [\"vendor/**\"]\nrules:\n  - {id: a, kind: banned-literal, literal: X, message: m}\nallow:\n  - {rule: a, paths: [\"docs/**\"], reason: a long enough reason}\n",
		"rules/more.yaml": "version: 1\nrules:\n  - {id: b, kind: retired-term, term: Old, message: m}\n",
	})
	rs, err := Load(dir, []string{"rules/main.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.Rules) != 2 || len(rs.Allow) != 1 || len(rs.Skip) != 1 || strings.Join(rs.Files, ",") != "rules/main.yaml,rules/more.yaml" {
		t.Fatalf("%+v", rs)
	}
}

func TestLoadRefusesWithFileAndKey(t *testing.T) {
	good := "  - {id: a, kind: banned-literal, literal: X, message: m}\n"
	cases := []struct {
		name, body, key string
	}{
		{"unknown key", "version: 1\nrules:\n  - {id: a, kind: banned-literal, literal: X, message: m, colour: red}\n", "colour"},
		{"bad version", "version: 2\nrules: []\n", "version"},
		{"bad kind", "version: 1\nrules:\n  - {id: a, kind: other, message: m}\n", "rules[0].kind"},
		{"no message", "version: 1\nrules:\n  - {id: a, kind: banned-literal, literal: X}\n", "rules[0].message"},
		{"bad id", "version: 1\nrules:\n  - {id: A b, kind: banned-literal, literal: X, message: m}\n", "rules[0].id"},
		{"literal missing", "version: 1\nrules:\n  - {id: a, kind: banned-literal, message: m}\n", "rules[0].literal"},
		{"term on a literal rule", "version: 1\nrules:\n  - {id: a, kind: banned-literal, literal: X, term: Y, message: m}\n", "rules[0].term"},
		{"plain host in a dead-host rule", "version: 1\nrules:\n  - {id: a, kind: dead-host, literal: " + plantedName() + ", message: m}\n", "rules[0].sha256"},
		{"short hash", "version: 1\nrules:\n  - {id: a, kind: dead-host, sha256: [abc], message: m}\n", "rules[0].sha256[0]"},
		{"upper hash", "version: 1\nrules:\n  - {id: a, kind: dead-host, sha256: [" + strings.ToUpper(HashLiteral("x")) + "], message: m}\n", "rules[0].sha256[0]"},
		{"duplicate id", "version: 1\nrules:\n" + good + good, "rules[1].id"},
		{"allow without reason", "version: 1\nrules:\n" + good + "allow:\n  - {rule: a, paths: [x], reason: short}\n", "allow[0].reason"},
		{"allow without paths", "version: 1\nrules:\n" + good + "allow:\n  - {rule: a, reason: a long enough reason}\n", "allow[0].paths"},
		{"allow for an unknown rule", "version: 1\nrules:\n" + good + "allow:\n  - {rule: zzz, paths: [x], reason: a long enough reason}\n", "allow[0].rule"},
		{"bad glob", "version: 1\nrules:\n  - {id: a, kind: banned-literal, literal: X, message: m, paths: [\"[\"]}\n", "rules[0].paths[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{"r.yaml": tc.body})
			_, err := Load(dir, []string{"r.yaml"})
			mustErr(t, err, "r.yaml", tc.key)
		})
	}
}

func TestLoadRefusesUnreadableAndEscapingFiles(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"loop-a.yaml": "version: 1\ninclude: [loop-b.yaml]\nrules: []\n",
		"loop-b.yaml": "version: 1\ninclude: [loop-a.yaml]\nrules: []\n",
		"inc.yaml":    "version: 1\ninclude: [missing.yaml]\nrules: []\n",
	})
	_, err := Load(dir, []string{"nope.yaml"})
	mustErr(t, err, "nope.yaml", "")
	for _, f := range []string{"../x.yaml", "/etc/passwd", ""} {
		if _, err := Load(dir, []string{f}); err == nil {
			t.Errorf("%q must be refused", f)
		}
	}
	_, err = Load(dir, []string{"loop-a.yaml"})
	if e, ok := err.(*Error); !ok || e.Key != "include" || !strings.Contains(e.Msg, "cycle") {
		t.Errorf("cycle: %v", err)
	}
	_, err = Load(dir, []string{"inc.yaml"})
	mustErr(t, err, "inc.yaml", "include[0]")
}

func TestGlob(t *testing.T) {
	for _, tc := range []struct {
		pat, rel string
		want     bool
	}{
		{"CHANGELOG*", "CHANGELOG.md", true},
		{"CHANGELOG*", "docs/CHANGELOG-2025.md", true},
		{"CHANGELOG*", "docs/notes.md", false},
		{"docs/**", "docs/a/b.md", true},
		{"docs/**", "other/docs/a.md", false},
		{"**/testdata/**", "x/y/testdata/z/f.txt", true},
		{"**/testdata/**", "testdata/f.txt", true},
		{"a/*.md", "a/b/c.md", false},
		{"a/**/c.md", "a/c.md", true},
		{"a/**/c.md", "a/x/y/c.md", true},
	} {
		if got := matchGlob(tc.pat, tc.rel); got != tc.want {
			t.Errorf("matchGlob(%q, %q) = %v", tc.pat, tc.rel, got)
		}
	}
}
