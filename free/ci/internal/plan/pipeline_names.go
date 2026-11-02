package plan

import (
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// sortedPipelineNames returns deterministic pipeline names without mutating cfg.
func sortedPipelineNames(cfg model.PipelineConfig) []string {
	names := make([]string, 0, len(cfg.Pipelines))
	for n := range cfg.Pipelines {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
