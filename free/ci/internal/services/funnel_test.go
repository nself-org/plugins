package services

// Purpose: the production Runner keeps its exec-funnel state out of the job
// home: a job's orphan sweep reads Home/processes and must never see (and
// kill) a service call another job has in flight (P7-CI-42 review).

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFunnelKeepsProcessMarkersOutOfTheJobHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("service dependencies need a POSIX host")
	}
	home := t.TempDir()
	f := &Funnel{Home: home}
	res, err := f.Run(context.Background(), Call{Argv: []string{"sh", "-c", "sleep 0.3; ls " + filepath.Join(home, funnelDir, "processes") + "; ls " + home}})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("run: %v (exit %d): %s", err, res.ExitCode, res.Output)
	}
	if !strings.Contains(res.Output, "svccall-") {
		t.Fatalf("the call's marker is not under Home/%s/processes:\n%s", funnelDir, res.Output)
	}
	for _, line := range strings.Split(res.Output, "\n") {
		if strings.TrimSpace(line) == "processes" || strings.TrimSpace(line) == "work" {
			t.Fatalf("the funnel wrote %q directly under the job home:\n%s", line, res.Output)
		}
	}
}
