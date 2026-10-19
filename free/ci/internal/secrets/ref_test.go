package secrets

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		in, class, env, name, file string
	}{
		{"project/FOO", "project", "", "FOO", "ci"},
		{"team/T_1", "team", "", "T_1", ""},
		{"release/ANDROID_KEY_ALIAS", "release", "", "ANDROID_KEY_ALIAS", "release"},
		{"environment/staging/DB_URL", "environment", "staging", "DB_URL", "ci-staging"},
		{"deploy/prod-1/TOKEN", "deploy", "prod-1", "TOKEN", "prod-1"},
	} {
		r, err := Parse(tc.in)
		if err != nil || string(r.Class) != tc.class || r.Env != tc.env || r.Name != tc.name || r.EnvName() != tc.name || r.StoreFile() != tc.file {
			t.Errorf("%s: %+v %v", tc.in, r, err)
		}
	}
}

func TestParseRejects(t *testing.T) {
	long := "project/A" + strings.Repeat("B", 128)
	for _, in := range []string{
		"", "FOO", "project", "project/", "project/foo", "project/1FOO", "project/FOO/BAR", "project/FOO-BAR", "secret/FOO",
		"environment/FOO", "environment/ci/FOO", "environment/release/FOO", "environment/team/FOO", "environment/ci-x/FOO",
		"environment/Prod/FOO", "environment/-x/FOO", "environment/" + strings.Repeat("a", 33) + "/FOO", "deploy/prod/", "deploy/ci/X",
		"team/foo", "release/FOO/BAR", " project/FOO", "project/FOO\n", long, "project/F\x00O",
	} {
		_, err := Parse(in)
		if CodeOf(err) != "E674" {
			t.Errorf("%q: want E674, got %v", in, err)
		}
	}
}

func TestParseAllCollision(t *testing.T) {
	_, err := ParseAll([]string{"project/FOO", "release/FOO"})
	if CodeOf(err) != "E674" || !strings.Contains(err.Error(), "FOO") {
		t.Fatalf("want E674 naming FOO, got %v", err)
	}
	refs, err := ParseAll([]string{"project/FOO", "release/BAR", "environment/dev/BAZ"})
	if err != nil || len(refs) != 3 {
		t.Fatal(err)
	}
	if cs := Classes(refs); len(cs) != 3 {
		t.Fatalf("classes %v", cs)
	}
}

func TestBadRefErrorIsBounded(t *testing.T) {
	_, err := Parse("project/" + strings.Repeat("x", 500) + "\x07")
	if len(err.Error()) > 200 || strings.ContainsRune(err.Error(), 7) {
		t.Fatalf("unbounded or unprintable echo: %q", err.Error())
	}
}

func TestParseClass(t *testing.T) {
	for in, want := range map[string]string{"project": "ci", "release": "release", "environment/dev": "ci-dev", "deploy/prod": "prod"} {
		c, env, err := ParseClass(in)
		if err != nil || StoreFileOf(c, env) != want {
			t.Errorf("%s: %v %q %v", in, c, env, err)
		}
	}
	for _, in := range []string{"", "team", "none", "project/x", "release/x", "environment", "environment/ci", "deploy/", "deploy/Prod"} {
		if _, _, err := ParseClass(in); CodeOf(err) != "E674" {
			t.Errorf("%q: want E674, got %v", in, err)
		}
	}
}

// FuzzParseRef: Parse never panics, and an accepted ref always satisfies the grammar and maps to a store file.
func FuzzParseRef(f *testing.F) {
	for _, s := range []string{"project/FOO", "environment/dev/X", "deploy/p/Y", "release/Z", "team/T", "", "//", "project/\xff", "environment/ci/A"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		r, err := Parse(s)
		if err != nil {
			if CodeOf(err) != "E674" || len(err.Error()) > 220 {
				t.Fatalf("bad error %q", err.Error())
			}
			return
		}
		if !namePattern.MatchString(r.Name) || r.Raw != s {
			t.Fatalf("accepted %q as %+v", s, r)
		}
		if r.Class != "team" && r.StoreFile() == "" {
			t.Fatalf("no store file for %q", s)
		}
		if r.Env != "" && (reservedEnv(r.Env) || !envPattern.MatchString(r.Env)) {
			t.Fatalf("accepted env %q", r.Env)
		}
	})
}

func TestReservedNamesAreRefused(t *testing.T) {
	for _, n := range []string{"PATH", "HOME", "DOCKER_HOST", "DOCKER_CONFIG", "LD_PRELOAD", "DYLD_INSERT_LIBRARIES", "GIT_SSH_COMMAND", "BASH_ENV", "NSELF_CI_HOME", "SECRETS_AGE_KEY_PATH"} {
		if _, err := Parse("project/" + n); CodeOf(err) != "E674" {
			t.Fatalf("%s accepted: %v", n, err)
		}
	}
	if _, err := Parse("project/NPM_TOKEN"); err != nil {
		t.Fatal(err)
	}
}
