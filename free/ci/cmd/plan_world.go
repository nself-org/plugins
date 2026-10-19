package main

// Purpose: the local world `nself ci plan` places jobs in (platform, tools on
// PATH, Docker, CPU and memory), split from plan.go to keep each file small.
// Constraints: a tool counts only when it is on PATH and runs; no nodes or
// providers exist here (closed world).

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

var planToolAlias = map[string]string{"xcode": "xcodebuild"}

// planWorld probes the local world: platform, the tools the plan may need,
// Docker, CPU and memory. No nodes or providers exist yet (closed world). With
// container (--container) the tools are those of the pinned toolchain image,
// not the host PATH: the jobs run in the image.
func planWorld(cfg model.PipelineConfig, checks []plan.Check, container bool) plan.World {
	w := plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH}
	inImage := map[string]bool{}
	if container {
		tools, err := toolchain.ImageTools()
		if err != nil {
			container = false // an unreadable table falls back to the host probe; the run itself reports it
		}
		for _, t := range tools {
			inImage[t] = true
		}
	}
	for _, t := range plan.WantedTools(cfg, checks) {
		switch {
		case t == "":
		case container:
			if inImage[t] {
				w.Tools = append(w.Tools, t)
			}
		case planOnPath(firstOf(planToolAlias[t], t)) && planToolRuns(firstOf(planToolAlias[t], t)):
			w.Tools = append(w.Tools, t)
		}
	}
	w.Docker = planOnPath("docker")
	c := ciexec.Probe()
	w.CPU, w.MemMB = c.CPUs, c.MemoryMB
	return w
}

// planToolRuns is true unless the executable on PATH is a stub: the Command
// Line Tools put an xcodebuild there that exits non-zero without Xcode.
func planToolRuns(name string) bool {
	if name != "xcodebuild" {
		return true
	}
	_, ok := planRun(context.Background(), "xcodebuild", "-version")
	return ok
}

func planOnPath(name string) bool {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			dir = "."
		}
		if s, err := os.Stat(filepath.Join(dir, name)); err == nil && !s.IsDir() && s.Mode()&0111 != 0 {
			return true
		}
	}
	return false
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
