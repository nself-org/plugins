package specs

// Purpose: detect private dependencies (Epic CACHE D5, R-SEC-18): an
// authenticated .npmrc or .yarnrc.yml, GOPRIVATE/GONOSUMDB, a credentialed
// cargo registry or pip/uv index. The result feeds `private_deps` on the
// entry, which switches ro-snapshot serving off (authz.SnapshotEligible).
// Inputs:  the workspace directories, the user's home, the environment.
// Outputs: PrivateDeps (a bool and the names of the signals, never values).
// Constraints: conservative; any signal counts, an unreadable file counts as a
// signal, and no value of a file or variable is returned or logged.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PrivInput is what PrivateDeps inspects.
type PrivInput struct {
	Dirs    []string // workspace root and workdir
	Home    string   // the user's home ("" skips user-level files)
	Environ []string // NAME=VALUE entries the job will see
}

var (
	npmAuth     = regexp.MustCompile(`(?m)^\s*[^#;\n]*(_authToken|_auth|_password|:username)\s*=`)
	yarnAuth    = regexp.MustCompile(`(?m)^\s*(npmAuthToken|npmAuthIdent)\s*:`)
	urlCred     = regexp.MustCompile(`://[^/\s@:]+:[^/\s@]*@`)
	cargoReg    = regexp.MustCompile(`(?m)^\s*\[(registries\.|source\.)[^\]]*\]|^\s*(token|credential-provider)\s*=`)
	privateEnvs = []string{"GOPRIVATE", "GONOSUMDB", "GONOPROXY", "GONOSUMCHECK", "NPM_TOKEN", "NODE_AUTH_TOKEN", "npm_config__authtoken",
		"npm_config__auth", "CARGO_REGISTRY_TOKEN", "UV_INDEX_USERNAME", "UV_INDEX_PASSWORD"}
	urlEnvs = []string{"PIP_INDEX_URL", "PIP_EXTRA_INDEX_URL", "UV_INDEX_URL", "UV_EXTRA_INDEX_URL", "npm_config_registry"}
)

// PrivateDeps reports whether the job's dependency setup is authenticated or
// private, and the names of the signals that said so.
func PrivateDeps(in PrivInput) (bool, []string) {
	var why []string
	add := func(s string) { why = append(why, s) }
	for _, kv := range in.Environ {
		name, v, _ := strings.Cut(kv, "=")
		for _, p := range privateEnvs {
			if strings.EqualFold(name, p) && v != "" {
				add("env:" + name)
			}
		}
		for _, p := range urlEnvs {
			if strings.EqualFold(name, p) && urlCred.MatchString(v) {
				add("env:" + name)
			}
		}
		if strings.HasPrefix(name, "CARGO_REGISTRIES_") && v != "" {
			add("env:" + name)
		}
		if name == "GOFLAGS" && strings.Contains(v, "-insecure") {
			add("env:GOFLAGS")
		}
	}
	dirs := append([]string(nil), in.Dirs...)
	if in.Home != "" {
		dirs = append(dirs, in.Home)
	}
	seen := map[string]bool{}
	for _, d := range dirs {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		scanFile(d, ".npmrc", npmAuth, add)
		scanFile(d, ".yarnrc.yml", yarnAuth, add)
		scanFile(d, filepath.Join(".cargo", "config.toml"), cargoReg, add)
		scanFile(d, filepath.Join(".cargo", "config"), cargoReg, add)
		scanFile(d, "pip.conf", urlCred, add)
		scanFile(d, filepath.Join(".config", "pip", "pip.conf"), urlCred, add)
		scanFile(d, "uv.toml", urlCred, add)
		for _, f := range []string{"requirements.txt", "requirements-dev.txt", "pyproject.toml"} {
			scanFile(d, f, urlCred, add)
		}
		for _, f := range []string{filepath.Join(".cargo", "credentials.toml"), filepath.Join(".cargo", "credentials"), ".netrc"} {
			if d == in.Home && exists(filepath.Join(d, f)) {
				add("file:" + f)
			}
		}
	}
	return len(why) > 0, why
}

// scanFile adds a signal when the file matches re or cannot be read (but exists).
func scanFile(dir, name string, re *regexp.Regexp, add func(string)) {
	p := filepath.Join(dir, name)
	b, err := os.ReadFile(p)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		add("unreadable:" + name)
	case re.Match(b):
		add("file:" + name)
	}
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }
