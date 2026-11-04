package exec

// Purpose: StepShim (D-1230), the seam that starts a process job step through the job shim chain
// (`sudo -n -u nself-ci-job-<n> <agent> job-shim` on a system node) instead of as the agent.
// Inputs:  the payload argv an Isolator moved out of JobSpec.Command, the step's workdir and full
//          environment (job env, secrets, NSELF_CI_EXEC_ATTEMPT), the job's own stdin.
// Outputs: the environment the chain itself starts with and a stdin that begins with one JSON request
//          line ({"argv","dir","env"}, the shape `nself-ci-agent job-shim` reads) followed by the job's stdin.
// Constraints: the payload never becomes an argv the agent execs: Command holds only the chain, so a
//          chain that fails (sudo refused, shim refused) fails the step and nothing runs as the agent's
//          UID. The payload's command is resolved here with the job's PATH, because the shim resolves a
//          bare name with sudo's reset PATH. The step's environment travels on the pipe, never in argv.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// StepShim is set by an Isolator that runs process jobs as another user. Command is then the chain
// (for example sudo ... job-shim) and Payload is what the chain runs.
type StepShim struct {
	// Payload is the job's own argv (an `env K=V` prefix is folded into the environment, as for a direct start).
	Payload []string
	// Env is the environment of the chain process itself; the job's environment goes in the request.
	Env []string
}

// shimLine is the request line; its fields match cmd/agent's shimRequest.
type shimLine struct {
	Argv []string `json:"argv"`
	Dir  string   `json:"dir"`
	Env  []string `json:"env"`
}

// launch returns the chain's environment and stdin for one step.
func (sh *StepShim) launch(workdir string, env []string, stdin io.Reader) ([]string, io.Reader, error) {
	if len(sh.Payload) == 0 || sh.Payload[0] == "" {
		return nil, nil, coded("E610", "empty command")
	}
	argv, jobEnv := foldEnvPrefix(sh.Payload, env)
	path, err := resolveGatedPath(argv[0], workdir, jobEnv)
	if err != nil {
		return nil, nil, err
	}
	for _, e := range jobEnv {
		if strings.IndexByte(e, 0) >= 0 {
			return nil, nil, errors.New("exec: environment variable contains NUL")
		}
	}
	line, err := json.Marshal(shimLine{Argv: append([]string{path}, argv[1:]...), Dir: workdir, Env: jobEnv})
	if err != nil {
		return nil, nil, err
	}
	req := bytes.NewReader(append(line, '\n'))
	if stdin == nil {
		return append([]string{}, sh.Env...), req, nil
	}
	return append([]string{}, sh.Env...), io.MultiReader(req, stdin), nil
}
