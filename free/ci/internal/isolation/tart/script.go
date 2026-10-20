package tart

// Purpose: the one script the ssh session reads on stdin: environment, working directory and the job command.
// Inputs:  the job's declared env, the network scope's proxy variables, the resolved secrets, the command.
// Outputs: script bytes built lazily on the first read (after the executor resolved the secrets) and zeroed
//          once read.
// Constraints: no secret and no job value ever appears on an argv; every value is single-quoted for POSIX sh;
//          the command is the last line and replaces the shell (exec), so nothing after it can be read as
//          script and the job's stdin is the end of the stream.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

// shQuote single-quotes s for POSIX sh.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// exportLines renders `export NAME='value'` for each pair, sorted by name.
func exportLines(env map[string]string) (string, error) {
	names := make([]string, 0, len(env))
	for k := range env {
		if !envName.MatchString(k) {
			return "", fmt.Errorf("environment name %q cannot be exported into the VM", k)
		}
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, k := range names {
		if strings.ContainsRune(env[k], 0) {
			return "", fmt.Errorf("environment value of %s contains a NUL byte", k)
		}
		b.WriteString("export " + k + "=" + shQuote(env[k]) + "\n")
	}
	return b.String(), nil
}

// workdirIn returns the remote working directory below the attempt's work dir.
func workdirIn(base, workdir string) (string, error) {
	if workdir == "" || workdir == "." {
		return base, nil
	}
	c := path.Clean(workdir)
	if path.IsAbs(c) || c == ".." || strings.HasPrefix(c, "../") {
		return "", errors.New("workdir escapes the workspace")
	}
	return path.Join(base, c), nil
}

// remoteBase is the attempt's directory in the VM, relative to the ssh user's home.
func remoteBase(attempt string) string { return "work/" + attempt }

// capturingSource passes resolution through and keeps a copy of the values so the script can carry them.
type capturingSource struct {
	inner  exec.SecretSource
	mu     sync.Mutex
	values map[string]string
}

func (c *capturingSource) Resolve(ctx context.Context, refs []string) (map[string]string, error) {
	m, err := c.inner.Resolve(ctx, refs)
	c.mu.Lock()
	c.values = make(map[string]string, len(m))
	for k, v := range m {
		c.values[k] = v
	}
	c.mu.Unlock()
	return m, err
}

// take returns the captured values and forgets them.
func (c *capturingSource) take() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.values
	c.values = nil
	return v
}

// buildScript assembles the stdin script. secretValues come from the capturing source; env is the job env
// plus the scope's proxy variables.
func buildScript(command []string, workdir string, env, secretValues map[string]string) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("set -e\n")
	lines, err := exportLines(env)
	if err != nil {
		return nil, err
	}
	b.WriteString(lines)
	if len(secretValues) > 0 {
		sec, err := secrets.TartStdin(secretValues)
		if err != nil {
			return nil, err
		}
		b.Write(sec)
	}
	b.WriteString("cd -- " + shQuote(workdir) + "\n")
	b.WriteString("exec")
	for _, arg := range command {
		if strings.ContainsRune(arg, 0) {
			return nil, errors.New("command argument contains a NUL byte")
		}
		b.WriteString(" " + shQuote(arg))
	}
	b.WriteString("\n")
	return b.Bytes(), nil
}

// lazyReader builds its content on the first Read and zeroes it after the last.
type lazyReader struct {
	build func() ([]byte, error)
	once  sync.Once
	data  []byte
	pos   int
	err   error
}

func (r *lazyReader) Read(p []byte) (int, error) {
	r.once.Do(func() { r.data, r.err = r.build() })
	if r.err != nil {
		return 0, r.err
	}
	if r.pos >= len(r.data) {
		for i := range r.data {
			r.data[i] = 0
		}
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
