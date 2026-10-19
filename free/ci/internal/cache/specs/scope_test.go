package specs

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/detect"
)

func TestCheckPathsRefusesPathsOutsideTheWorkspace(t *testing.T) {
	for _, p := range []string{"/abs", "../x", "a/../../x", "..", "a\\b", "a\x00b", "a\nb", ""} {
		_, err := CheckPaths([]string{"ok", p})
		var r *entries.Refused
		if !errors.As(err, &r) || r.Reason != entries.ReasonPathOutside {
			t.Errorf("%q: err %v", p, err)
		}
	}
	got, err := CheckPaths([]string{"svc/api/", "./svc/web", "svc/api", "."})
	if err != nil || strings.Join(got, ",") != ".,svc/api,svc/web" {
		t.Fatalf("got %v err %v", got, err)
	}
}

func TestNarrowKeepsFactsInsideTheScope(t *testing.T) {
	facts := []detect.Fact{fact("stack", "go", "go.mod"), fact("stack", "go", "svc/api/go.mod"), fact("stack", "go", "svc/apix/go.mod"), fact("stack", "go", "svc/web/go.mod")}
	cases := map[string]string{"": "4", "svc/api": "svc/api/go.mod", "svc": "svc/api/go.mod,svc/apix/go.mod,svc/web/go.mod", ".": "4", "other": ""}
	for scope, want := range cases {
		var sc []string
		if scope != "" {
			sc = []string{scope}
		}
		var paths []string
		for _, f := range Narrow(facts, sc) {
			paths = append(paths, f.Path)
		}
		got := strings.Join(paths, ",")
		if want == "4" {
			want = strings.Join([]string{"go.mod", "svc/api/go.mod", "svc/apix/go.mod", "svc/web/go.mod"}, ",")
		}
		if got != want {
			t.Errorf("scope %q: %s, want %s", scope, got, want)
		}
	}
}

func TestPrivateDepsSignals(t *testing.T) {
	write := func(t *testing.T, dir, name, body string) {
		t.Helper()
		putFile(t, filepath.Join(dir, name), body)
	}
	for name, c := range map[string]struct {
		file, body string
		env        []string
		home       bool
	}{
		"npmrc auth token":      {file: ".npmrc", body: "//registry.example.invalid/:_authToken=${NPM_TOKEN}\n"},
		"npmrc without auth":    {file: ".npmrc", body: "registry=https://registry.example.invalid/\nsave-exact=true\n"},
		"npmrc url creds":       {file: ".npmrc", body: "registry=https://u:p@registry.example.invalid/\n"},
		"yarnrc token":          {file: ".yarnrc.yml", body: "npmRegistries:\n  x:\n    npmAuthToken: abc\n"},
		"cargo registry":        {file: ".cargo/config.toml", body: "[registries.corp]\nindex = \"sparse+https://cargo.example.invalid/\"\n"},
		"requirements cred url": {file: "requirements.txt", body: "--index-url https://u:p@pypi.example.invalid/simple\n"},
		"go private env":        {env: []string{"GOPRIVATE=example.invalid/*"}},
		"pip index with creds":  {env: []string{"PIP_INDEX_URL=https://u:p@example.invalid/simple"}},
		"pip index without":     {env: []string{"PIP_INDEX_URL=https://example.invalid/simple"}},
		"cargo registry env":    {env: []string{"CARGO_REGISTRIES_CORP_TOKEN=abc"}},
		"netrc in the home":     {file: ".netrc", body: "machine example.invalid\n", home: true},
		"nothing":               {},
	} {
		dir, home := t.TempDir(), t.TempDir()
		if c.file != "" {
			if c.home {
				write(t, home, c.file, c.body)
			} else {
				write(t, dir, c.file, c.body)
			}
		}
		got, why := PrivateDeps(PrivInput{Dirs: []string{dir}, Home: home, Environ: c.env})
		want := strings.Contains(name, "auth token") || strings.Contains(name, "url creds") || strings.Contains(name, "token") || strings.Contains(name, "cargo registry") || strings.Contains(name, "cred url") ||
			strings.Contains(name, "go private") || strings.Contains(name, "with creds") || strings.Contains(name, "netrc")
		if got != want {
			t.Errorf("%s: %v (%v), want %v", name, got, why, want)
		}
		for _, w := range why {
			if strings.Contains(w, "registry.example") || strings.Contains(w, "abc") {
				t.Errorf("%s: signal %q leaks a value", name, w)
			}
		}
	}
}
