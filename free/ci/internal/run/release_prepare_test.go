package run

// Purpose: D-1077. The deploy-credential check of a release job runs in
// prepare, after the cache and services hooks added their environment, so a
// caller-supplied Hooks.Isolation cannot bypass it.

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type envCache map[string]string

func (e envCache) Before(context.Context, AttemptInfo) (CacheSetup, error) {
	return CacheSetup{Env: e}, nil
}
func (envCache) After(context.Context, AttemptInfo, Outcome) error { return nil }

type envServices map[string]string

func (e envServices) Up(context.Context, ServicesRequest) (ServicesSetup, error) {
	return ServicesSetup{Env: e}, nil
}
func (envServices) Down(context.Context, ServicesRequest) error { return nil }

func TestPrepareRefusesDeployCredentialsAddedByHooks(t *testing.T) {
	for name, tc := range map[string]struct {
		h        Hooks
		services []string
	}{
		"cache hook":    {Hooks{Cache: envCache{"DEPLOY_TOKEN": "x"}}, nil},
		"services hook": {Hooks{Services: envServices{"nself_deploy_key": "x"}}, []string{"db"}},
	} {
		rs := &runState{c: &Coordinator{}, hooks: tc.h}
		_, err := rs.prepare(context.Background(), &AttemptInfo{RunID: "r", Job: model.Job{Kind: "release", NeedsServices: tc.services}})
		if err == nil || CodeOf(err) != "E715" || !strings.Contains(err.Error(), "deploy_env") {
			t.Errorf("%s: a release job got a deploy credential: %v", name, err)
		}
		if _, err = rs.prepare(context.Background(), &AttemptInfo{RunID: "r", Job: model.Job{Kind: "test", NeedsServices: tc.services}}); err != nil {
			t.Errorf("%s: a non-release job was refused: %v", name, err)
		}
	}
}
