package exec

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestScrubRepoEnvDropsRepositoryVariablesOnly(t *testing.T) {
	for _, n := range []string{"GIT_DIR", "GIT_INDEX_FILE", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0"} {
		t.Setenv(n, "x")
	}
	t.Setenv("GIT_SSH_COMMAND", "ssh -i k")
	t.Setenv("NSELF_SCRUB_KEEP", "1")
	env, err := ScrubRepoEnv{}.Environment(context.Background(), model.Job{})
	if err != nil {
		t.Fatal(err)
	}
	joined := "\n" + strings.Join(env, "\n")
	for _, gone := range []string{"GIT_DIR=", "GIT_INDEX_FILE=", "GIT_CONFIG_PARAMETERS=", "GIT_CONFIG_COUNT=", "GIT_CONFIG_KEY_0=", "GIT_CONFIG_VALUE_0="} {
		if strings.Contains(joined, "\n"+gone) {
			t.Errorf("%s must be removed", gone)
		}
	}
	for _, kept := range []string{"GIT_SSH_COMMAND=", "NSELF_SCRUB_KEEP="} {
		if !strings.Contains(joined, "\n"+kept) {
			t.Errorf("%s must be kept", kept)
		}
	}
}
