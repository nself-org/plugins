package tart

// Purpose: the one script the ssh session reads on stdin: environment, working directory and the job command.
// Inputs:  the job's declared env, the network scope's proxy variables, the resolved secrets, the command.
// Outputs: script bytes built lazily on the first read (after the executor resolved the secrets) and zeroed
//          once read.
// Constraints: no secret and no job value ever appears on an argv; every value is single-quoted for POSIX sh;
//          the command is the last line and replaces the shell (exec), so nothing after it can be read as
//          script and the job's stdin is the end of the stream; with a status prefix it runs in a subshell
//          with stdin /dev/null and its exit code is printed on a line of its own after it.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
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
	// The executor puts the map in the host ssh process's environment under these names. They are renamed to a
	// harmless fixed pattern so a secret named like a loader variable (DYLD_*) cannot load code into ssh; the
	// real names travel only in the script. The values are still registered with the redactor by the executor.
	hidden := make(map[string]string, len(m))
	i := 0
	for k, v := range m {
		c.values[k] = v
		hidden[fmt.Sprintf("NSELF_CI_VMSECRET_%d", i)] = v
		i++
	}
	c.mu.Unlock()
	if err != nil {
		return m, err
	}
	return hidden, nil
}

// take returns the captured values and forgets them.
func (c *capturingSource) take() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.values
	c.values = nil
	return v
}

// buildScript assembles the stdin script with the command as its last line (exec). secretValues come from the
// capturing source; env is the job env plus the scope's proxy variables.
func buildScript(command []string, workdir string, env, secretValues map[string]string) ([]byte, error) {
	return buildScriptStatus(command, workdir, env, secretValues, "")
}

// buildScriptStatus is buildScript with a status line: a non-empty statusPrefix runs the command in a subshell
// (stdin /dev/null, as exec left it) and prints `<statusPrefix><exit code>` after it, so the host can tell the
// job's own exit 255 from a failed ssh session (D-1355).
func buildScriptStatus(command []string, workdir string, env, secretValues map[string]string, statusPrefix string) ([]byte, error) {
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
	if statusPrefix == "" {
		b.WriteString("exec")
	} else {
		b.WriteString("rc=0\n( exec")
	}
	for _, arg := range command {
		if strings.ContainsRune(arg, 0) {
			return nil, errors.New("command argument contains a NUL byte")
		}
		b.WriteString(" " + shQuote(arg))
	}
	if statusPrefix == "" {
		b.WriteString("\n")
	} else {
		b.WriteString(" ) </dev/null || rc=$?\nprintf '\\n%s%d\\n' " + shQuote(statusPrefix) + " \"$rc\"\nexit \"$rc\"\n")
	}
	return b.Bytes(), nil
}

// sshFailure is the exit code ssh itself uses for a connection or protocol error.
const sshFailure = 255

// exitStatus watches the job output for the status line the script prints once the command ended, takes it out
// of the stream and passes the rest on unchanged. The line is `\n<prefix><code>\n`; the prefix carries a
// per-attempt nonce, so job output cannot forge it. Bytes that might start the line are held back until the
// next chunk shows they do not.
type exitStatus struct {
	prefix string
	next   func([]byte)
	mu     sync.Mutex
	hold   []byte
	code   int
	have   bool
}

func newExitStatus(next func([]byte)) (*exitStatus, error) {
	var n [8]byte
	if _, err := rand.Read(n[:]); err != nil {
		return nil, err
	}
	return &exitStatus{prefix: "nself-ci:exit:" + hex.EncodeToString(n[:]) + ":", next: next}, nil
}

// write is the job's Output.
func (e *exitStatus) write(b []byte) {
	e.mu.Lock()
	out := e.filter(b)
	e.mu.Unlock()
	if len(out) > 0 && e.next != nil {
		e.next(out)
	}
}

func (e *exitStatus) filter(b []byte) []byte {
	buf := append(e.hold, b...)
	e.hold = nil
	lead := []byte("\n" + e.prefix)
	var out []byte
	for {
		i := bytes.Index(buf, lead)
		if i < 0 {
			break
		}
		out = append(out, buf[:i]...)
		rest := buf[i+len(lead):]
		j := bytes.IndexByte(rest, '\n')
		if j < 0 { // the line is not whole yet
			e.hold = append([]byte(nil), buf[i:]...)
			return out
		}
		if n, err := strconv.Atoi(string(rest[:j])); err == nil {
			e.code, e.have = n, true
		}
		buf = rest[j+1:]
	}
	keep := 0 // the longest tail of buf that is a proper prefix of lead
	for k := min(len(lead)-1, len(buf)); k > 0; k-- {
		if bytes.HasPrefix(lead, buf[len(buf)-k:]) {
			keep = k
			break
		}
	}
	out = append(out, buf[:len(buf)-keep]...)
	e.hold = append([]byte(nil), buf[len(buf)-keep:]...)
	return out
}

// flush passes on the held-back bytes (they turned out not to be a status line) and reports the status.
func (e *exitStatus) flush() (int, bool) {
	e.mu.Lock()
	rest, code, have := e.hold, e.code, e.have
	e.hold = nil
	e.mu.Unlock()
	if len(rest) > 0 && e.next != nil {
		e.next(rest)
	}
	return code, have
}

// afterExit is the executor's AfterExit hook: ssh exit 255 without a status line saying the job itself exited
// 255 is a failed session, an infra failure, never the job's result.
func (a *attempt) afterExit(exit int) error {
	code, ok := a.status.flush()
	if exit != sshFailure || (ok && code == sshFailure) {
		return nil
	}
	return fmt.Errorf("the ssh session to %s failed (exit %d) before the job reported its own exit status", a.name, exit)
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
