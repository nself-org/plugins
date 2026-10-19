package logs_test

// P7-CI-39 end to end: a real run through the coordinator stores each
// attempt's output behind the redactor, a planted secret reaches no stored
// byte, and the stored log reads back whole.

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/logs"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/run"
)

const testSHA = "0123456789abcdef0123456789abcdef01234567"

type okReporter struct{}

func (okReporter) Post(context.Context, report.Status) error { return nil }

func fakeGit(_ context.Context, argv ...string) (string, int, error) {
	switch j := strings.Join(argv, " "); {
	case strings.Contains(j, "rev-parse HEAD"):
		return testSHA + "\n", 0, nil
	case strings.Contains(j, "remote get-url"):
		return "git@github.com:acme/widgets.git\n", 0, nil
	}
	return "", 0, nil
}

func yes() *bool { b := true; return &b }

func runJobs(t *testing.T, home string, jobs map[string]model.Job) run.Result {
	t.Helper()
	root := t.TempDir()
	pl, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: jobs},
		World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		t.Fatal(err)
	}
	c := &run.Coordinator{Opts: run.Options{Root: root, Home: home}, Plan: pl, ConfigDigest: "cfg", Out: io.Discard, Capture: fakeGit,
		Reporter: okReporter{}, Poll: 10 * time.Millisecond,
		Capacity: func() ciexec.Capacity {
			return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
		}}
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func sh(script string) model.Job {
	return model.Job{Kind: "test", Run: []string{"sh", "-c", script}, Required: yes()}
}

// storedBytes returns every byte under dir, segments unzipped.
func storedBytes(t *testing.T, dir string) string {
	t.Helper()
	var all bytes.Buffer
	_ = filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(p)
		if strings.HasSuffix(p, ".gz") {
			if zr, zerr := gzip.NewReader(bytes.NewReader(b)); zerr == nil {
				b, _ = io.ReadAll(zr)
			}
		}
		all.Write(b)
		return nil
	})
	return all.String()
}

func TestRunStoresRedactedLogsPerAttempt(t *testing.T) {
	const secret = "e2e-log-secret-3d9f60aa17"
	if err := run.RegisterSecretValue([]byte(secret)); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	script := `echo first line; printf 'raw=%s\n' ` + secret + `; printf 'b64='; printf %s ` + secret + ` | base64; echo;` +
		` printf 'split=e2e-log-'; sleep 0.2; printf 'secret-3d9f60aa17\n'; echo last line; echo oops >&2`
	res := run.Result{}
	res = runJobs(t, home, map[string]model.Job{"lint": sh(script), "quiet": sh("true")})
	var o run.JobOutcome
	for _, j := range res.Summary.Jobs {
		if j.ID == "lint" {
			o = j
		}
	}
	if o.AttemptID == "" {
		t.Fatalf("no attempt id in %+v", res.Summary.Jobs)
	}
	dir := logs.AttemptDir(home, res.Summary.RunID, o.AttemptID)
	var lines []logs.Line
	if err := logs.Read(dir, 0, func(l logs.Line) error { lines = append(lines, l); return nil }); err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var text []string
	for i, l := range lines {
		if l.Seq != int64(i+1) || l.Stream != "out" || l.TS == "" {
			t.Fatalf("line %d: %+v", i, l)
		}
		text = append(text, l.Text)
	}
	joined := strings.Join(text, "\n")
	for _, want := range []string{"first line", "last line", "oops", redact.Placeholder} {
		if !strings.Contains(joined, want) {
			t.Errorf("stored log lacks %q:\n%s", want, joined)
		}
	}
	all := storedBytes(t, filepath.Join(home, "logs"))
	b64 := base64.StdEncoding.EncodeToString([]byte(secret))[:20]
	for _, bad := range []string{secret, b64, "e2e-log-", "secret-3d9f60aa17"} {
		if strings.Contains(all, bad) {
			t.Fatalf("a planted value reached the stored logs (%q)", bad)
		}
	}
	if runs, _ := logs.ListRuns(home); len(runs) != 1 {
		t.Fatalf("log dirs: %+v", runs)
	}
}
