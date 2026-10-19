package adhoc

// Purpose: the minimal environment of an ad-hoc job, at every isolation level.
// Inputs:  the operator's PATH and LANG, the scratch directory (process level).
// Outputs: KEY=VALUE entries: PATH, LANG, TMPDIR and HOME, nothing else.
// Constraints: the host environment is never inherited (no token, no cloud
// credential, no SSH agent socket). A process gets HOME and TMPDIR inside the
// scratch directory the handler creates and removes. A container gets the
// image's own PATH (the host PATH means nothing there) and the tmpfs /tmp the
// hardened profile mounts; job-declared env (NSELF_CI_*, service connection
// variables) is added by the executor after this.

import (
	"context"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// envPolicy implements exec.EnvPolicy.
type envPolicy struct {
	container bool
	scratch   string // process level: HOME and TMPDIR
	getenv    func(string) string
}

func (p envPolicy) Environment(context.Context, model.Job) ([]string, error) {
	get := p.getenv
	if get == nil {
		get = os.Getenv
	}
	lang := get("LANG")
	if lang == "" {
		lang = "C.UTF-8"
	}
	if p.container {
		return []string{"LANG=" + lang, "HOME=/tmp", "TMPDIR=/tmp"}, nil
	}
	env := []string{"LANG=" + lang, "HOME=" + p.scratch, "TMPDIR=" + p.scratch}
	if path := get("PATH"); path != "" {
		env = append(env, "PATH="+path)
	}
	return env, nil
}
