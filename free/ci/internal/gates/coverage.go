package gates

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

var goCover = regexp.MustCompile(`(?m)^total:\s+\(statements\)\s+([0-9.]+)%`)

func applyCoverage(c *model.Check, root string, p PlannedCheck, cfg model.PipelineConfig, res invocation) {
	setting, ok := cfg.Checks[p.Def.ID]
	if !ok || setting.CoverageFloor == nil {
		c.Result = "skip"
		c.Excerpt = "coverage floor not configured"
		return
	}
	floor := *setting.CoverageFloor
	if floor < 0 || floor > 100 {
		c.Result = "error"
		c.Excerpt = "E615: coverage floor must be 0..100"
		return
	}
	percent, found := 0.0, false
	if p.Def.ID == "go.coverage" {
		if match := goCover.FindStringSubmatch(res.output); len(match) > 1 {
			percent, _ = strconv.ParseFloat(match[1], 64)
			found = true
		}
	}
	if p.Def.ID == "node.coverage" {
		percent, found = lcov(filepath.Join(root, filepath.FromSlash(p.Workdir), "coverage", "lcov.info"))
	}
	if !found {
		c.Result = "error"
		c.Excerpt = "E615: coverage report unavailable"
		return
	}
	if percent < float64(floor) {
		c.Result = "fail"
		c.FailureClass = "code"
		c.Excerpt = "coverage below configured floor"
	}
}

func lcov(name string) (float64, bool) {
	f, err := os.Open(name)
	if err != nil {
		return 0, false
	}
	defer func() { _ = f.Close() }()
	var found, hits int
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "LF:") {
			n, _ := strconv.Atoi(strings.TrimPrefix(line, "LF:"))
			found += n
		}
		if strings.HasPrefix(line, "LH:") {
			n, _ := strconv.Atoi(strings.TrimPrefix(line, "LH:"))
			hits += n
		}
	}
	if s.Err() != nil || found == 0 {
		return 0, false
	}
	return float64(hits) * 100 / float64(found), true
}
