package report

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CaptureAs answers for the job's workdir and environment, not an empty
// directory (D-1035).
func TestCaptureAsRunsInTheJobsWorkdirWithItsEnv(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "pin"), []byte("pinned-1.2.3"), 0o600); err != nil {
		t.Fatal(err)
	}
	job := model.Job{Kind: "static", Workdir: "sub", Env: map[string]string{"NSELF_CAP_PROBE": "from-job"}}
	out, code, err := CaptureAs(context.Background(), job, root, "sh", "-c", `cat pin; echo " $NSELF_CAP_PROBE"`)
	if err != nil || code != 0 || strings.TrimSpace(out) != "pinned-1.2.3 from-job" {
		t.Fatalf("out %q code %d err %v", out, code, err)
	}
	out, _, _ = Capture(context.Background(), "sh", "-c", `cat pin; echo " $NSELF_CAP_PROBE"`)
	if strings.Contains(out, "pinned-1.2.3") || strings.Contains(out, "from-job") {
		t.Fatalf("Capture must stay an empty workspace: %q", out)
	}
}
