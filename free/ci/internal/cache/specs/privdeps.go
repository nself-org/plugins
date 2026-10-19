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
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

// PrivInput is what PrivateDeps inspects.
type PrivInput struct {
	Dirs    []string // workspace root and workdir
	Home    string   // the user's home ("" skips user-level files)
	Environ []string // NAME=VALUE entries the job will see
}

var (
	npmAuth     = regexp.MustCompile(`(?m)^\s*[^#;\n]*(_authToken|_auth|_password|:username)\s*=|://[^/\s@:]+:[^/\s@]*@`)
	yarnAuth    = regexp.MustCompile(`["']?(npmAuthToken|npmAuthIdent)["']?\s*:`)
	urlCred     = regexp.MustCompile(`://[^/\s@:]+:[^/\s@]*@`)
	cargoReg    = regexp.MustCompile(`(?m)^\s*\[(registries\.|source\.)[^\]]*\]|^\s*(token|credential-provider)\s*=`)
	privateEnvs = []string{"GOPRIVATE", "GONOSUMDB", "GONOPROXY", "GONOSUMCHECK", "NPM_TOKEN", "NODE_AUTH_TOKEN", "npm_config__authtoken",
		"npm_config__auth", "CARGO_REGISTRY_TOKEN", "UV_INDEX_USERNAME", "UV_INDEX_PASSWORD", "GOAUTH"}
	urlEnvs = []string{"PIP_INDEX_URL", "PIP_EXTRA_INDEX_URL", "UV_INDEX_URL", "UV_EXTRA_INDEX_URL", "npm_config_registry", "GOPROXY"}
	goEnvRe = regexp.MustCompile(`(?m)^\s*(GOPRIVATE|GONOSUMDB|GONOPROXY|GONOSUMCHECK|GOAUTH)\s*=\s*\S|://[^/\s@:]+:[^/\s@]*@`)
)

// PrivateDeps reports whether the job's dependency setup is authenticated or
// private, and the names of the signals that said so.
func PrivateDeps(in PrivInput) (bool, []string) {
	var why []string
	add := func(s string) { why = append(why, s) }
	toolHomes := map[string]string{} // files the environment points the tools at, by variable
	for _, kv := range in.Environ {
		name, v, _ := strings.Cut(kv, "=")
		if v != "" {
			toolHomes[strings.ToUpper(name)] = v
		}
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
	for _, f := range []struct {
		env string
		re  *regexp.Regexp
	}{{"NPM_CONFIG_USERCONFIG", npmAuth}, {"NPM_CONFIG_GLOBALCONFIG", npmAuth}, {"PIP_CONFIG_FILE", urlCred}, {"UV_CONFIG_FILE", urlCred}} {
		if p := toolHomes[f.env]; p != "" {
			scanFile(filepath.Dir(p), filepath.Base(p), f.re, add)
		}
	}
	// `go env -w` keeps its settings in a go/env file the job's go reads: GOENV, else
	// $XDG_CONFIG_HOME/go/env, else the user's config directory
	var goenvs []string
	switch g := toolHomes["GOENV"]; {
	case g == "off":
	case g != "" && !filepath.IsAbs(g):
		add("env:GOENV") // a relative path is resolved against the job's directory: not followed, so private
	case g != "":
		goenvs = append(goenvs, g)
	default:
		if x := toolHomes["XDG_CONFIG_HOME"]; x != "" {
			goenvs = append(goenvs, filepath.Join(x, "go", "env"))
		}
		if in.Home != "" {
			goenvs = append(goenvs, filepath.Join(in.Home, ".config", "go", "env"), filepath.Join(in.Home, "Library", "Application Support", "go", "env"))
		}
	}
	for _, g := range goenvs {
		scanFile(filepath.Dir(g), filepath.Base(g), goEnvRe, add)
	}
	if ch := toolHomes["CARGO_HOME"]; ch != "" { // the cargo spec rebinds and seeds from this home
		for _, n := range []string{"config.toml", "config"} {
			scanFile(ch, n, cargoReg, add)
		}
		for _, n := range []string{"credentials.toml", "credentials"} {
			if exists(filepath.Join(ch, n)) {
				add("file:CARGO_HOME/" + n)
			}
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

// maxConfigBytes bounds what the scan reads of one configuration file.
const maxConfigBytes = 1 << 20

// scanFile adds a signal when the file matches re. A file that exists but is not
// a regular file within maxConfigBytes (a device, a FIFO, a huge or unreadable
// file, or one swapped for such between the check and the read) counts as a
// signal: it is opened non-blocking, checked on the open descriptor and read
// through a size limit.
func scanFile(dir, name string, re *regexp.Regexp, add func(string)) {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		add("unreadable:" + name)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxConfigBytes {
		add("unreadable:" + name)
		return
	}
	b, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	switch {
	case err != nil || len(b) > maxConfigBytes:
		add("unreadable:" + name)
	case re.Match(b):
		add("file:" + name)
	}
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }
