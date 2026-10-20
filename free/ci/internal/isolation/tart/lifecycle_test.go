package tart

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestRunInCloneAndDelete(t *testing.T) {
	r := newRig(t, nil)
	s := r.spec("a1", "sh", "-c", "echo hello-from-vm; echo $PWD > where.txt; test -f src.txt && cat src.txt")
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "src.txt"), []byte("workspace-file\n"), 0o600)
	s.SourceDir = src
	res, out := r.run(context.Background(), s)
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("run: %+v\n%s", res, out)
	}
	if !strings.Contains(out, "hello-from-vm") || !strings.Contains(out, "workspace-file") {
		t.Fatalf("output %q", out)
	}
	if strings.Contains(r.read("tart.log"), "run --no-graphics") == false {
		t.Fatalf("tart log: %s", r.read("tart.log"))
	}
	r.noClones()
}

func TestJobExitCodeAndFailurePathsDeleteTheClone(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		cmd  []string
		want func(res int, err error) bool
	}{
		{"job fails", nil, []string{"sh", "-c", "exit 7"}, func(c int, err error) bool { return c == 7 && err == nil }},
		{"clone fails", map[string]string{"FAKE_TART_FAIL": "clone"}, []string{"true"}, func(c int, err error) bool { return err != nil }},
		{"run exits at once", map[string]string{"FAKE_TART_FAIL": "run"}, []string{"true"}, func(c int, err error) bool { return err != nil }},
		{"no ip", map[string]string{"FAKE_TART_FAIL": "ip"}, []string{"true"}, func(c int, err error) bool { return err != nil }},
		{"ssh never ready", map[string]string{"FAKE_SSH_FAIL": "1"}, []string{"true"}, func(c int, err error) bool { return err != nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, nil)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			res, out := r.run(context.Background(), r.spec("f1", tc.cmd...))
			if !tc.want(res.ExitCode, res.Err) {
				t.Fatalf("result %+v\n%s", res, out)
			}
			r.noClones()
		})
	}
}

func TestCancelMidJobDeletesTheClone(t *testing.T) {
	r := newRig(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for i := 0; i < 100; i++ {
			if _, err := os.Stat(filepath.Join(r.dir, "vms", "nself-ci-c1", "home", "work", "c1")); err == nil {
				time.Sleep(300 * time.Millisecond)
				cancel()
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		cancel()
	}()
	res, _ := r.run(ctx, r.spec("c1", "sleep", "30"))
	if res.Attempt.State != "cancelled" {
		t.Fatalf("state %q err %v", res.Attempt.State, res.Err)
	}
	r.noClones()
}

func TestDeleteRefusedKeepsTheMarkerForTheSweep(t *testing.T) {
	r := newRig(t, nil)
	t.Setenv("FAKE_TART_FAIL", "delete")
	res, _ := r.run(context.Background(), r.spec("d1", "true"))
	if res.Err != nil {
		t.Fatalf("job should pass: %v", res.Err)
	}
	if _, err := os.Stat(markerPath(stateDir(r.home), CloneName("d1"))); err != nil {
		t.Fatalf("the marker must stay so the sweep can retry: %v", err)
	}
	// the operator fixes tart; the next Prepare's sweep removes the clone
	t.Setenv("FAKE_TART_FAIL", "")
	removed := r.p.Sweep(context.Background(), r.p.opts.NewTool("coord2", r.home), stateDir(r.home))
	if len(removed) != 1 || removed[0] != CloneName("d1") {
		t.Fatalf("sweep removed %v", removed)
	}
	r.noClones()
}

func TestSecretsReachTheVMOnStdinOnly(t *testing.T) {
	const planted = "s3cr3t-'$(touch pwned)'-VALUE"
	r := newRig(t, nil)
	s := r.spec("s1", "sh", "-c", `printf '%s' "$API_TOKEN" > got.txt; printf '%s' "$PLAIN" >> got.txt`)
	s.Job.Env = map[string]string{"PLAIN": "p-v"}
	s.Job.Secrets = []string{"project/API_TOKEN"}
	s.Secrets = fakeSecrets{"API_TOKEN": planted}
	res, out := r.run(context.Background(), s)
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("run: %+v %s", res, out)
	}
	for _, f := range []string{"tart.log", "ssh.log"} {
		if strings.Contains(r.read(f), "s3cr3t") || strings.Contains(r.read(f), "API_TOKEN") {
			t.Fatalf("%s carries the secret or its name:\n%s", f, r.read(f))
		}
	}
	r.noClones()
}

func TestSecretRoundTripInTheVM(t *testing.T) {
	const planted = "s3cr3t-'$(touch pwned)'-VALUE"
	r := newRig(t, nil)
	s := r.spec("s2", "sh", "-c", `printf '%s' "$API_TOKEN"`)
	s.Job.Secrets = []string{"project/API_TOKEN"}
	s.Secrets = fakeSecrets{"API_TOKEN": planted}
	res, out := r.run(context.Background(), s)
	if res.Err != nil || out != planted {
		t.Fatalf("job saw %q err %v", out, res.Err)
	}
	if _, err := os.Stat("pwned"); err == nil {
		t.Fatal("a shell metacharacter in a secret ran")
	}
}

func TestSecretsWithoutPerAttemptSourceAreRefused(t *testing.T) {
	r := newRig(t, nil)
	s := r.spec("s3", "true")
	s.Job.Secrets = []string{"project/X"}
	res, _ := r.run(context.Background(), s)
	if res.Err == nil || !res.Refused {
		t.Fatalf("want a refusal, got %+v", res)
	}
}

func TestNoFreeSlotRefusesBeforeAnythingIsCreated(t *testing.T) {
	r := newRig(t, nil)
	t.Setenv("FAKE_TART_EXTRA_RUNNING", "2")
	res, _ := r.run(context.Background(), r.spec("n1", "true"))
	if !res.Refused || res.Err == nil || !strings.Contains(res.Err.Error(), "E678") {
		t.Fatalf("want E678 refusal, got %+v", res)
	}
	r.noClones()
}

func TestArtifactsComeBackAndOnlyThose(t *testing.T) {
	r := newRig(t, nil)
	s := r.spec("o1", "sh", "-c", "mkdir -p dist && echo built > dist/app.txt && echo junk > other.txt")
	s.Job.Artifacts = []model.ArtifactDecl{{Path: "dist"}}
	ws := t.TempDir()
	var got string
	s.SourceDir = ws
	s.AfterJob = func(_ context.Context, root *os.Root) error {
		b, err := root.ReadFile("dist/app.txt")
		got = string(b)
		if _, e := root.Stat("other.txt"); e == nil {
			t.Error("an undeclared file came back")
		}
		return err
	}
	res, out := r.run(context.Background(), s)
	if res.Err != nil || got != "built\n" {
		t.Fatalf("res %+v got %q out %s", res, got, out)
	}
	r.noClones()
}

func TestMissingArtifactFailsTheAttempt(t *testing.T) {
	r := newRig(t, nil)
	s := r.spec("o2", "true")
	s.Job.Artifacts = []model.ArtifactDecl{{Path: "dist"}}
	res, _ := r.run(context.Background(), s)
	if res.Err == nil {
		t.Fatal("a declared output the job did not produce must fail")
	}
	r.noClones()
}

func TestHookOrderBeforeAndAfterJob(t *testing.T) {
	r := newRig(t, nil)
	var order []string
	s := r.spec("h1", "true")
	s.BeforeJob = func(context.Context, *os.Root) error { order = append(order, "before"); return nil }
	s.AfterJob = func(context.Context, *os.Root) error { order = append(order, "after"); return nil }
	s.Release = func(context.Context) { order = append(order, "release") }
	if res, _ := r.run(context.Background(), s); res.Err != nil {
		t.Fatal(res.Err)
	}
	if strings.Join(order, ",") != "before,after,release" {
		t.Fatalf("order %v", order)
	}
}
