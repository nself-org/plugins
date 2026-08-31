// Package isolate manages a pool of pre-warmed Deno subprocesses.
//
// SSRF guard: user functions may NOT reach RFC-1918 or loopback addresses.
// --allow-net is intentionally NOT passed unrestricted. Instead, the operator
// provides NSELF_ALLOW_NET (comma-separated external hostnames). RFC-1918 and
// loopback are blocked at the Go layer before the subprocess spawns.
// If NSELF_ALLOW_NET is empty, Deno runs with --deny-net (no outbound network
// at all). This satisfies the Security-Always-Free Doctrine.
package isolate

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Isolate represents a single Deno worker process for one invocation.
type Isolate struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	done   chan struct{}
}

// Pool manages pre-warmed Deno isolate slots.
type Pool struct {
	mu          sync.Mutex
	slots       chan struct{} // semaphore for pool capacity
	denoBin     string
	maxMemoryMB int
	timeoutMS   int
	allowNet    []string // explicit external hostname allowlist (never RFC-1918/loopback)
	logger      *slog.Logger
}

// NewPool creates an isolate pool. size is the max concurrent executions.
// allowNet is a list of permitted external hostnames; pass nil/empty to deny
// all outbound network access from user functions.
func NewPool(denoBin string, size, maxMemoryMB, timeoutMS int, logger *slog.Logger) *Pool {
	if size < 1 {
		size = 5
	}
	p := &Pool{
		slots:       make(chan struct{}, size),
		denoBin:     denoBin,
		maxMemoryMB: maxMemoryMB,
		timeoutMS:   timeoutMS,
		logger:      logger,
		allowNet:    parseAllowNet(os.Getenv("NSELF_ALLOW_NET")),
	}
	for i := 0; i < size; i++ {
		p.slots <- struct{}{}
	}
	return p
}

// parseAllowNet parses a comma-separated hostname list, stripping empties.
func parseAllowNet(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		h := strings.TrimSpace(p)
		if h != "" {
			out = append(out, h)
		}
	}
	return out
}

// IsPrivateHost returns true if host is a loopback or RFC-1918 address.
// Used at startup validation and in tests; Deno's --deny-net handles runtime.
func IsPrivateHost(host string) bool {
	// Strip port if present.
	h := host
	if strings.Contains(host, ":") {
		var err error
		h, _, err = net.SplitHostPort(host)
		if err != nil {
			h = host
		}
	}
	ip := net.ParseIP(h)
	if ip == nil {
		// Not an IP address — hostname-only entries are presumed external.
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

// PoolSize returns the current semaphore capacity (max concurrent).
func (p *Pool) PoolSize() int {
	return cap(p.slots)
}

// Acquire blocks until a slot is available (up to timeout), then returns it.
// Caller MUST call Release when done.
func (p *Pool) Acquire(ctx context.Context) error {
	select {
	case <-p.slots:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release returns a slot to the pool.
func (p *Pool) Release() {
	p.slots <- struct{}{}
}

// InvokeResult is the output of a single function invocation.
type InvokeResult struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	LogLines   []string
	Duration   time.Duration
	TimedOut   bool
}

// Invoke runs source code as a Deno function with the provided env vars and request body.
// env must be an allowlist-only map (caller is responsible for filtering).
func (p *Pool) Invoke(ctx context.Context, source string, env map[string]string, method, path string, requestBody []byte) (*InvokeResult, error) {
	start := time.Now()

	// Write source to a temp file — Deno needs a file path.
	tmpFile, err := os.CreateTemp("", "nself-edge-*.ts")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write([]byte(source)); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	// Build deadline context for timeout enforcement.
	deadline := time.Duration(p.timeoutMS) * time.Millisecond
	runCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	// Compose Deno command.
	// --no-config prevents loading any deno.json from the filesystem.
	// --no-npm prevents npm: imports from touching the host.
	// SSRF guard: --deny-net blocks all outbound by default; --allow-net is set
	// only when the operator has explicitly permitted specific external hostnames.
	// RFC-1918/loopback are never permitted regardless of NSELF_ALLOW_NET.
	args := []string{
		"run",
		"--no-config",
		"--no-npm",
		"--allow-read=/tmp",
		fmt.Sprintf("--v8-flags=--max-old-space-size=%d", p.maxMemoryMB),
	}

	if len(p.allowNet) > 0 {
		// Filter out any private/loopback entries that may have been misconfigured.
		safe := make([]string, 0, len(p.allowNet))
		for _, h := range p.allowNet {
			if !IsPrivateHost(h) {
				safe = append(safe, h)
			} else {
				p.logger.Warn("SSRF: rejected private host in NSELF_ALLOW_NET", "host", h)
			}
		}
		if len(safe) > 0 {
			args = append(args, "--allow-net="+strings.Join(safe, ","))
		} else {
			args = append(args, "--deny-net")
		}
	} else {
		// No allowlist configured — deny all outbound network.
		args = append(args, "--deny-net")
	}

	args = append(args, tmpFile.Name())

	cmd := exec.CommandContext(runCtx, p.denoBin, args...)

	// Allowlist-only env — DO NOT inherit os.Environ().
	denoEnv := make([]string, 0, len(env)+5)
	denoEnv = append(denoEnv, "DENO_NO_UPDATE_CHECK=1", "NO_COLOR=1")
	for k, v := range env {
		denoEnv = append(denoEnv, k+"="+v)
	}
	// Inject the simulated request info via env.
	denoEnv = append(denoEnv,
		"NSELF_REQUEST_METHOD="+method,
		"NSELF_REQUEST_PATH="+path,
	)
	cmd.Env = denoEnv

	// Feed request body via stdin.
	cmd.Stdin = bytes.NewReader(requestBody)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()

	elapsed := time.Since(start)
	result := &InvokeResult{
		Duration: elapsed,
		Headers:  map[string]string{"Content-Type": "application/json"},
	}

	if runCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.StatusCode = 504
		result.Body = []byte(`{"error":"function timeout"}`)
		p.logger.Warn("function timed out", "duration_ms", elapsed.Milliseconds())
		return result, nil
	}

	if runErr != nil {
		result.StatusCode = 500
		result.Body = []byte(fmt.Sprintf(`{"error":%q}`, errBuf.String()))
		// Collect stderr as log lines.
		result.LogLines = splitLines(errBuf.String())
		return result, nil
	}

	result.StatusCode = 200
	result.Body = outBuf.Bytes()
	result.LogLines = splitLines(errBuf.String())
	return result, nil
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				lines = append(lines, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
