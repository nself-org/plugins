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
