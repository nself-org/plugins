package exec

// Purpose: StepShim (D-1230), the seam that starts a process job step through the job shim chain
// (`sudo -n -u nself-ci-job-<n> <agent> job-shim` on a system node) instead of as the agent, and
// Stager, the seam that hands the step's workspace to the job user and takes the outputs back.
// Inputs:  the payload argv an Isolator moved out of JobSpec.Command, the step's workdir and full
//          environment (job env, secrets, NSELF_CI_EXEC_ATTEMPT), the job's own stdin; for a staged
//          step the directory the job user owns and the environment overrides that point into it.
// Outputs: the environment the chain itself starts with and a stdin that begins with one JSON request
//          line ({"argv","dir","env"}, the shape `nself-ci-agent job-shim` reads) followed by the job's stdin.
// Constraints: the payload never becomes an argv the agent execs: Command holds only the chain, so a
//          chain that fails (sudo refused, shim refused) fails the step and nothing runs as the agent's
//          UID. The payload's command is resolved here with the job's PATH, because the shim resolves a
//          bare name with sudo's reset PATH. The step's environment travels on the pipe, never in argv.
//          A staged step resolves its command in the agent's copy of the tree and runs it at the same
//          relative path in the job user's copy; a failed hand-off fails the step before anything starts.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
)

// StepShim is set by an Isolator that runs process jobs as another user. Command is then the chain
// (for example sudo ... job-shim) and Payload is what the chain runs.
type StepShim struct {
	// Payload is the job's own argv (an `env K=V` prefix is folded into the environment, as for a direct start).
	Payload []string
	// Env is the environment of the chain process itself; the job's environment goes in the request.
	Env []string
	// Stage, when set, moves the workspace to a directory the job user owns for the step; nil runs the
	// step in the agent's own workspace.
	Stage Stager
}

// Stager hands one attempt's workspace to the job user and back (D-1230). The agent's workspace stays
// the agent's: the job works on its own copy, which only the job user can enter.
type Stager interface {
	// StageIn copies the workspace into the job user's attempt directory. It returns the directory that
	// stands for the workspace root there and environment entries (HOME, TMPDIR) that replace the job's.
	StageIn(ctx context.Context, attempt, workspace string) (dir string, env []string, err error)
	// StageOut replaces the workspace's contents with the job user's copy after a step that passed, so
	// AfterJob collects what the job produced.
	StageOut(ctx context.Context, attempt, workspace string) error
}

// shimLine is the request line; its fields match cmd/agent's shimRequest.
type shimLine struct {
	Argv []string `json:"argv"`
	Dir  string   `json:"dir"`
	Env  []string `json:"env"`
}

// stage hands the workspace over when a Stager is set. It returns the directory the step runs in, the
// step's environment, and finish, which takes the outputs back after a step that passed.
func (sh *StepShim) stage(ctx context.Context, attempt, workspace, root, workdir string, env []string) (string, []string, func(passed bool) error, error) {
	none := func(bool) error { return nil }
	if sh.Stage == nil {
		return workdir, env, none, nil
	}
	rel, err := filepath.Rel(root, workdir)
	if err != nil || !filepath.IsLocal(rel) && rel != "." {
		return "", nil, none, coded("E612", "workdir escapes workspace")
	}
	dir, over, err := sh.Stage.StageIn(ctx, attempt, workspace)
	if err != nil {
		return "", nil, none, coded("E610", "job workspace hand-off failed: "+firstLine(err.Error()))
	}
	finish := func(passed bool) error {
		if !passed {
			return nil // nothing is collected from a failed step; hygiene removes the job user's copy
		}
		if err := sh.Stage.StageOut(context.WithoutCancel(ctx), attempt, workspace); err != nil {
			return coded("E610", "job workspace hand-back failed: "+firstLine(err.Error()))
		}
		return nil
	}
	return filepath.Join(dir, rel), replaceEnv(env, over), finish, nil
}

// replaceEnv sets each KEY=VALUE of over in env, dropping every earlier entry of the same key.
func replaceEnv(env, over []string) []string {
	drop := map[string]bool{}
	for _, e := range over {
		k, _, _ := strings.Cut(e, "=")
		drop[k] = true
	}
	out := make([]string, 0, len(env)+len(over))
	for _, e := range env {
		if k, _, _ := strings.Cut(e, "="); !drop[k] {
			out = append(out, e)
		}
	}
	return append(out, over...)
}

// launch returns the chain's environment and stdin for one step. The command is resolved in workdir
// (the agent's tree); a path inside it is moved to the same place under runDir (the job user's tree).
func (sh *StepShim) launch(workdir, runDir string, env []string, stdin io.Reader) ([]string, io.Reader, error) {
	if len(sh.Payload) == 0 || sh.Payload[0] == "" {
		return nil, nil, coded("E610", "empty command")
	}
	argv, jobEnv := foldEnvPrefix(sh.Payload, env)
	path, err := resolveGatedPath(argv[0], workdir, jobEnv)
	if err != nil {
		return nil, nil, err
	}
	if runDir != workdir {
		if rel, rerr := filepath.Rel(workdir, path); rerr == nil && (filepath.IsLocal(rel) || rel == ".") {
			path = filepath.Join(runDir, rel)
		}
	}
	for _, e := range jobEnv {
		if strings.IndexByte(e, 0) >= 0 {
			return nil, nil, errors.New("exec: environment variable contains NUL")
		}
	}
	line, err := json.Marshal(shimLine{Argv: append([]string{path}, argv[1:]...), Dir: runDir, Env: jobEnv})
	if err != nil {
		return nil, nil, err
	}
	req := bytes.NewReader(append(line, '\n'))
	if stdin == nil {
		return append([]string{}, sh.Env...), req, nil
	}
	return append([]string{}, sh.Env...), io.MultiReader(req, stdin), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
