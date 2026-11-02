package plan

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestNamedPipelineHonorsExplicitTrigger(t *testing.T) {
	for _, tt := range []struct {
		name     string
		triggers []model.Trigger
		trigger  model.Trigger
		wantErr  bool
	}{
		{"release rejects pull request", []model.Trigger{"tag"}, "pull_request", true},
		{"release rejects push", []model.Trigger{"tag"}, "push", true},
		{"release accepts tag", []model.Trigger{"tag"}, "tag", false},
		{"accepts one matching trigger", []model.Trigger{"push", "manual"}, "manual", false},
		{"empty triggers reject explicit trigger", nil, "pull_request", true},
		{"manual selection without trigger", []model.Trigger{"tag"}, "", false},
		{"explicit local must match", []model.Trigger{"tag"}, "local", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			release := job()
			release.Kind = "release"
			cfg := cfgOf(map[string]model.Job{"release": release})
			cfg.Pipelines = map[string]model.PipelineDef{
				"release": {Triggers: tt.triggers, Jobs: []string{"release"}},
			}
			p, err := Build(Input{Config: cfg, Pipeline: "release", Trigger: tt.trigger, World: linuxWorld()})
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "does not accept trigger "+string(tt.trigger)) {
					t.Fatalf("Build = %v, %v; want trigger mismatch", p, err)
				}
				if p != nil {
					t.Fatal("trigger mismatch returned executable plan")
				}
				return
			}
			if err != nil || find(p, "release") == nil {
				t.Fatalf("Build = %v, %v; want release job", p, err)
			}
		})
	}
}
