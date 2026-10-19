package exec

// Purpose: the default environment a job gets from the coordinator: a fixed
// allowlist of the operator's variables, never the whole environment.
// Inputs:  the process environment.
// Outputs: KEY=VALUE entries for the names that are safe to hand a job.
// Constraints: a name is allowed only if listed or under an allowed prefix;
// job-declared env and resolved secrets are added by the executor after this.

import (
	"context"
	"os"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

var allowedEnvNames = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SHELL": true, "LANG": true, "LANGUAGE": true,
	"TERM": true, "COLORTERM": true, "NO_COLOR": true, "TZ": true, "TMPDIR": true, "TEMP": true, "TMP": true, "CI": true,
	"GOPATH": true, "GOROOT": true, "GOCACHE": true, "GOMODCACHE": true, "GOFLAGS": true, "GOOS": true, "GOARCH": true,
	"GOTOOLCHAIN": true, "GOPRIVATE": true, "GONOSUMDB": true, "GONOPROXY": true, "GOSUMDB": true,
	"CARGO_HOME": true, "RUSTUP_HOME": true, "RUSTFLAGS": true, "PNPM_HOME": true, "NVM_DIR": true,
	// Windows tools fail without these.
	"SYSTEMROOT": true, "SYSTEMDRIVE": true, "COMSPEC": true, "PATHEXT": true, "WINDIR": true, "USERPROFILE": true,
	"APPDATA": true, "LOCALAPPDATA": true, "PROGRAMFILES": true, "PROGRAMDATA": true,
}

var allowedEnvPrefixes = []string{"LC_", "XDG_", "CGO_"}

// AllowlistEnv is the default EnvPolicy: it passes only allowlisted names.
type AllowlistEnv struct{}

// Environment returns the allowlisted part of the process environment.
func (AllowlistEnv) Environment(_ context.Context, _ model.Job) ([]string, error) {
	var out []string
	for _, item := range os.Environ() {
		name, _, ok := strings.Cut(item, "=")
		if ok && envAllowed(name) {
			out = append(out, item)
		}
	}
	return out, nil
}

func envAllowed(name string) bool {
	name = strings.ToUpper(name)
	if allowedEnvNames[name] {
		return true
	}
	for _, p := range allowedEnvPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// repoEnvNames are the variables git uses to find or reconfigure a
// repository. A hook exports them for its own repository, so a capture that
// runs `git -C <root>` must not inherit them.
var repoEnvNames = map[string]bool{
	"GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_INDEX_FILE": true, "GIT_COMMON_DIR": true,
	"GIT_OBJECT_DIRECTORY": true, "GIT_ALTERNATE_OBJECT_DIRECTORIES": true, "GIT_NAMESPACE": true,
	"GIT_PREFIX": true, "GIT_QUARANTINE_PATH": true, "GIT_CONFIG_PARAMETERS": true, "GIT_CONFIG_COUNT": true,
}

// ScrubRepoEnv is an EnvPolicy that inherits the process environment minus the
// variables that name or reconfigure a git repository (GIT_DIR,
// GIT_INDEX_FILE, GIT_CONFIG_PARAMETERS, ...). Other GIT_* names pass.
type ScrubRepoEnv struct{}

// Environment returns the process environment without repository variables.
func (ScrubRepoEnv) Environment(_ context.Context, _ model.Job) ([]string, error) {
	var out []string
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		name = strings.ToUpper(name)
		if repoEnvNames[name] || strings.HasPrefix(name, "GIT_CONFIG_KEY_") || strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}
