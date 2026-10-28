package exec

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestMountArgsBindReadOnlyAndReadWrite(t *testing.T) {
	t.Parallel()
	got := mountArgs([]Mount{{Host: "/cache/go", Target: "/cache/go", ReadOnly: true}, {Host: "/cache/out", Target: "/out"}})
	want := []string{
		"--mount", "type=bind,src=/cache/go,dst=/cache/go,readonly",
		"--mount", "type=bind,src=/cache/out,dst=/out",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mount args\n got %q\nwant %q", got, want)
	}
}

func TestCheckMountsRefusesUnsafe(t *testing.T) {
	t.Parallel()
	for name, m := range map[string]Mount{
		"relative host":    {Host: "cache", Target: "/c"},
		"relative target":  {Host: "/cache", Target: "c"},
		"comma in host":    {Host: "/cache,bind-propagation=shared", Target: "/c"},
		"comma in target":  {Host: "/cache", Target: "/c,readonly=false"},
		"newline":          {Host: "/cache\n", Target: "/c"},
		"docker socket":    {Host: "/var/run/docker.sock", Target: "/c"},
		"podman socket":    {Host: "/run/user/1000/podman/podman.sock", Target: "/c"},
		"host root":        {Host: "/", Target: "/c"},
		"run dir":          {Host: "/var/run", Target: "/c"},
		"root target":      {Host: "/cache", Target: "/"},
		"workspace target": {Host: "/cache", Target: "/repo/sub"},
		"unclean target":   {Host: "/cache", Target: "/a/../repo"},
		"proc target":      {Host: "/cache", Target: "/proc/x"},
	} {
		err := checkMounts(JobSpec{Job: model.Job{Isolation: "container"}, Mounts: []Mount{m}})
		var ee *Error
		if !errors.As(err, &ee) || ee.Code != "E612" {
			t.Errorf("%s: err %v, want E612", name, err)
		}
	}
	dup := []Mount{{Host: "/a", Target: "/c"}, {Host: "/b", Target: "/c"}}
	if err := checkMounts(JobSpec{Job: model.Job{Isolation: "container"}, Mounts: dup}); err == nil {
		t.Error("duplicate target accepted")
	}
}

// The gVisor profile runs through the same container CLI, so it holds a read-only bind (the credential
// wrapper's agent binary) exactly as the baseline container does; only a process job refuses one.
func TestCheckMountsAcceptsReadOnlyForBothContainerLevels(t *testing.T) {
	t.Parallel()
	for _, level := range []model.Isolation{"container", "sandboxed-container"} {
		if err := checkMounts(JobSpec{Job: model.Job{Isolation: level}, Mounts: []Mount{{Host: t.TempDir(), Target: "/usr/local/bin/agent", ReadOnly: true}}}); err != nil {
			t.Errorf("%s: read-only mount refused: %v", level, err)
		}
	}
	if err := checkMounts(JobSpec{Job: model.Job{Isolation: "process"}, Mounts: []Mount{{Host: t.TempDir(), Target: "/a", ReadOnly: true}}}); err == nil {
		t.Error("process job accepted a read-only mount")
	}
}

// A process job cannot be held read-only, so a read-only mount must refuse
// the job instead of silently handing it a writable host path.
func TestProcessJobRefusesReadOnlyMount(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	r := (&Executor{Isolation: BaselineIsolator{}}).Run(t.Context(), JobSpec{AttemptID: "ro-process", CoordinatorID: "coord", Home: home,
		Mounts: []Mount{{Host: home, Target: "/cache", ReadOnly: true}}, Command: []string{"true"}})
	var ee *Error
	if !errors.As(r.Err, &ee) || ee.Code != "E611" || !strings.Contains(ee.Message, "read-only") {
		t.Fatalf("err %v, want E611 read-only refusal", r.Err)
	}
	ok := (&Executor{Isolation: BaselineIsolator{}}).Run(t.Context(), JobSpec{AttemptID: "rw-process", CoordinatorID: "coord", Home: home,
		Mounts: []Mount{{Host: home, Target: "/cache"}}, Command: []string{"true"}})
	if ok.Err != nil || ok.Attempt.State != "passed" {
		t.Fatalf("read-write mount on a process job: %+v", ok)
	}
}

func TestMountSourceCanonicalizedAndMissingSourceRefused(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}
	mounts := []Mount{{Host: alias, Target: "/cache", ReadOnly: true}}
	if err := checkMounts(JobSpec{Job: model.Job{Isolation: "container"}, Mounts: mounts}); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(source)
	if err != nil || mounts[0].Host != canonical {
		t.Fatalf("source=%q want=%q err=%v", mounts[0].Host, canonical, err)
	}
	mounts[0].Host = filepath.Join(dir, "missing")
	if err := checkMounts(JobSpec{Job: model.Job{Isolation: "container"}, Mounts: mounts}); err == nil {
		t.Fatal("missing mount source accepted")
	}
}
