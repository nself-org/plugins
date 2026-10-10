package gates

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

var sequence atomic.Uint64

// Runner invokes checks and probes exclusively through the local exec funnel.
type Runner struct {
	Executor *exec.Executor
	Home     string
}

// Run records a check result. Required missing tools fail closed with E614.
func (r Runner) Run(ctx context.Context, root string, p PlannedCheck, cfg model.PipelineConfig) model.Check {
	d := p.Def
	c := model.Check{ID: d.ID, JobID: d.ID, Kind: d.Kind, Tool: d.Tool, Required: p.Required, Substantive: d.Substantive}
	argv := append([]string(nil), d.Argv...)
	for i, a := range argv {
		argv[i] = strings.ReplaceAll(a, "{target}", p.Target)
	}
	if strings.HasPrefix(d.ID, "node.") {
		var reason string
		argv, reason = nodeCommand(root, p, cfg)
		if reason != "" {
			if reason == "unsupported package manager" {
				c.Result = "error"
				c.FailureClass = "infra"
				c.Excerpt = "E614: " + reason + "; " + d.Remediation
				return c
			}
			c.Result = "skip"
			c.Required = false
			c.Excerpt = reason
			return c
		}
		c.Tool = argv[0]
		if !available(argv[0]) {
			return unavailable(c, d.Remediation)
		}
	} else if !available(d.Tool) {
		return unavailable(c, d.Remediation)
	}
	probeTool := c.Tool
	if d.ID == "node.coverage" {
		if runtime.GOOS == "windows" {
			argv = []string{"cmd", "/C", probeTool + " run test -- --coverage && type coverage\\lcov.info"}
		} else {
			argv = []string{"sh", "-c", probeTool + " run test -- --coverage && cat coverage/lcov.info"}
		}
	}
	if d.ID == "secrets.gitleaks" {
		argv = gitleaksCommand(root)
	}
	if d.ID == "go.coverage" {
		if runtime.GOOS == "windows" {
			argv = []string{"cmd", "/C", "go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out"}
		} else {
			argv = []string{"sh", "-c", "go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out"}
		}
	}
	probe := d.VersionProbe
	if strings.HasPrefix(d.ID, "node.") {
		probe = []string{probeTool, "--version"}
	}
	version := r.invoke(ctx, root, probe, ".", 30*time.Second)
	if version.err == nil && version.exit == 0 {
		v := strings.TrimSpace(version.output)
		if len(v) > 120 {
			v = v[:120]
		}
		c.ToolVersion = &v
	}
	res := r.invoke(ctx, root, argv, p.Workdir, 0)
	if d.ID == "go.fmt" {
		res.output = dropVendored(res.output)
	}
	c.Excerpt = strings.TrimSpace(res.output)
	if len(c.Excerpt) > 1024 {
		c.Excerpt = c.Excerpt[:1024]
	}
	if res.err != nil {
		c.Result = "error"
		c.FailureClass = "infra"
		c.Excerpt = res.err.Error()
		return c
	}
	if advisoryOffline(d.ID, res.output, res.exit) {
		if !p.Required {
			c.Result = "skip"
			c.Reason = "advisory.data_offline"
			return c
		}
	}
	c.Findings = findings(d.ID, res.output)
	if res.exit != 0 || d.ID == "go.fmt" && strings.TrimSpace(res.output) != "" || c.Findings > 0 {
		c.Result = "fail"
		c.FailureClass = "code"
	} else {
		c.Result = "pass"
	}
	if d.ID == "go.coverage" || d.ID == "node.coverage" {
		applyCoverage(&c, root, p, cfg, res)
	}
	return c
}

func unavailable(c model.Check, fix string) model.Check {
	if c.Required {
		c.Result = "error"
		c.FailureClass = "infra"
		c.Excerpt = "E614: " + c.Tool + " unavailable; " + fix
	} else {
		c.Result = "skip"
		c.Reason = "advisory.tool_missing"
		c.Excerpt = c.Tool + " unavailable; " + fix
	}
	return c
}

func available(name string) bool {
	if name == "" || strings.Contains(name, "/") {
		return false
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			dir = "."
		}
		p := filepath.Join(dir, name)
		s, e := os.Stat(p)
		if e == nil && !s.IsDir() && s.Mode()&0111 != 0 {
			return true
		}
	}
	return false
}

type invocation struct {
	output string
	exit   int
	err    error
}

func (r Runner) invoke(ctx context.Context, root string, argv []string, workdir string, timeout time.Duration) invocation {
	if r.Executor == nil {
		return invocation{err: fmt.Errorf("executor required")}
	}
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	var b strings.Builder
	id := fmt.Sprintf("gate-%d", sequence.Add(1))
	res := r.Executor.Run(ctx, exec.JobSpec{AttemptID: id, CoordinatorID: "gates", Job: model.Job{Kind: "quality", Workdir: workdir, Isolation: "process"}, Command: argv, SourceDir: root, Home: r.Home, Timeout: timeout, Output: func(p []byte) { b.Write(p) }})
	return invocation{output: b.String(), exit: res.ExitCode, err: res.Err}
}
