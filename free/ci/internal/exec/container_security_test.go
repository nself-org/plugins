package exec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func fakeDocker(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\n"+body), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func TestContainerRejectsOptionShapedImage(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "invoked")
	fakeDocker(t, fmt.Sprintf("touch %q\nexit 0\n", marker))
	for _, image := range []string{"--privileged", "--pid=host", "bad image", "../evil"} {
		r := (&Executor{}).Run(context.Background(), JobSpec{AttemptID: "image", CoordinatorID: "coord", Home: t.TempDir(), Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: image}}, Command: []string{"true"}})
		if r.Err == nil {
			t.Fatalf("accepted image %q", image)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("docker invoked for invalid image %q", image)
		}
	}
}

func TestDockerCLIEnvironmentIsHostOnly(t *testing.T) {
	dir := t.TempDir()
	envPath, filePath := filepath.Join(dir, "host.env"), filepath.Join(dir, "job.env")
	fakeDocker(t, fmt.Sprintf("env > %q\nfor a in \"$@\"; do if [ \"$prev\" = --env-file ]; then cp \"$a\" %q; fi; prev=\"$a\"; done\nexit 0\n", envPath, filePath))
	r := (&Executor{}).Run(context.Background(), JobSpec{AttemptID: "host-env", CoordinatorID: "coord", Home: t.TempDir(), Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: "alpine:3.19"}, Env: map[string]string{"LD_PRELOAD": "evil.so", "DOCKER_HOST": "tcp://evil", "JOB_SECRET": "secret"}}, Command: []string{"true"}})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	host, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	job, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"LD_PRELOAD=", "DOCKER_HOST=", "JOB_SECRET="} {
		if strings.Contains(string(host), key) {
			t.Fatalf("host docker env contains %s", key)
		}
		if !strings.Contains(string(job), key) {
			t.Fatalf("container env missing %s", key)
		}
	}
}

func TestContainerHonorsWorkdir(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	fakeDocker(t, fmt.Sprintf("printf '%%s\\n' \"$@\" > %q\n", argsPath))
	r := (&Executor{}).Run(context.Background(), JobSpec{AttemptID: "workdir", CoordinatorID: "coord", Home: t.TempDir(), Job: model.Job{Isolation: "container", Workdir: "sub", Container: &model.ContainerConfig{Image: "alpine:3.19"}}, Command: []string{"pwd"}})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--workdir=/repo/sub") {
		t.Fatalf("args: %s", args)
	}
}

func TestContainerDotWorkdirUsesRepo(t *testing.T) {
	for _, dir := range []string{"", "."} {
		t.Run(fmt.Sprintf("workdir=%q", dir), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "args")
			fakeDocker(t, fmt.Sprintf("printf '%%s\\n' \"$@\" > %q\n", path))
			r := (&Executor{}).Run(context.Background(), JobSpec{AttemptID: "dot-container", CoordinatorID: "coord", Home: t.TempDir(), Job: model.Job{Isolation: "container", Workdir: dir, Container: &model.ContainerConfig{Image: "alpine:3.19"}}, Command: []string{"pwd"}})
			args, err := os.ReadFile(path)
			if r.Err != nil || err != nil || !strings.Contains(string(args), "--workdir=/repo\n") {
				t.Fatalf("run=%+v args=%q read=%v", r, args, err)
			}
		})
	}
}

func TestDockerStopBoundedByGrace(t *testing.T) {
	fakeDocker(t, "if [ \"$1\" = run ]; then sleep 30; fi\nif [ \"$1\" = stop ]; then sleep 30; fi\n")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	r := (&Executor{Grace: 100 * time.Millisecond}).Run(ctx, JobSpec{AttemptID: "stop", CoordinatorID: "coord", Home: t.TempDir(), Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: "alpine:3.19"}}, Command: []string{"sleep", "30"}})
	if r.Err == nil || time.Since(start) > 2500*time.Millisecond {
		t.Fatalf("unbounded stop: %+v elapsed=%s", r, time.Since(start))
	}
}

func TestOrphanContainerVanishedDuringSweep(t *testing.T) {
	fakeDocker(t, "echo 'Error: No such container: gone' >&2\nexit 1\n")
	if err := removeStaleContainer(context.Background(), "gone"); err != nil {
		t.Fatal(err)
	}
}
