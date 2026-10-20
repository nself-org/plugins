package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestCacheLocalitySourceUsesRestoreFacts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test\n\ngo 1.26\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                string
		job                 model.Job
		container, disabled bool
	}{
		{name: "process"},
		{name: "container", container: true},
		{name: "disabled", job: model.Job{Cache: &model.JobCacheConfig{Disable: true}}, disabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j := tc.job
			j.Secrets = []string{"release/KEY"}
			if tc.container {
				j.Container = &model.ContainerConfig{Image: "image@sha256:" + strings.Repeat("a", 64)}
			}
			in, err := CacheLocalitySource(&store.Store{}, store.PipelineRow{Project: root, SourceTrust: "owner"}, []AttemptInfo{{AttemptID: "build[linux]", JobName: "build", Job: j, Container: tc.container}})(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if tc.disabled {
				if len(in.Jobs) != 0 {
					t.Fatal("disabled cache contributed")
				}
				return
			}
			if len(in.Jobs) != 1 {
				t.Fatalf("want one locality job, got %d", len(in.Jobs))
			}
			got := in.Jobs[0]
			if got.Key != "build[linux]" || !got.Facts.SecretsDeclared || got.Facts.Trust != "owner" || got.Facts.Project != root || len(got.Detected) == 0 || len(got.ExtraKeys) != 1 || got.ExtraKeys[0] != "job=build" || got.Container != tc.container {
				t.Fatal("locality facts do not match the restore inputs")
			}
			if tc.container && got.Image != "sha256:"+strings.Repeat("a", 64) {
				t.Fatalf("unpinned image: %q", got.Image)
			}
		})
	}
}
