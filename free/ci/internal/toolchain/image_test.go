package toolchain

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const pinnedDigest = "sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func pinnedTable() string {
	return "version: 1\nrows:\n  - name: toolchain-image\n    kind: image\n    purpose: p\n    owner: o\n    image: docker.io/nself/ci-toolchain\n    version: docker.io/nself/ci-toolchain@" + pinnedDigest + "\n    status: pinned\n    digest: " + pinnedDigest + "\n    upstream: {type: none, ref: ''}\n"
}

func pendingTable() string {
	return "version: 1\nrows:\n  - name: toolchain-image\n    kind: image\n    purpose: p\n    owner: o\n    image: docker.io/nself/ci-toolchain\n    status: pending\n    upstream: {type: none, ref: ''}\n"
}

func TestPendingRowIsE645NamingImage(t *testing.T) {
	r := &Resolver{
		Getenv:   envOf(map[string]string{EnvVersions: "/fixture/versions.yaml"}),
		ReadFile: func(string) ([]byte, error) { return []byte(pendingTable()), nil },
	}
	_, err := r.Resolve(context.Background())
	if err == nil || !strings.HasPrefix(err.Error(), "E645: ") || !strings.Contains(err.Error(), "--image <ref@digest>") {
		t.Fatalf("pending row error = %v", err)
	}
}

func TestOverrideMustBePinnedByDigest(t *testing.T) {
	for _, bad := range []string{"alpine:3.22", "alpine@sha256:abc", "docker.io/library/alpine:latest", "ALPINE@" + pinnedDigest, "a b@" + pinnedDigest} {
		r := &Resolver{Getenv: envOf(map[string]string{EnvImage: bad})}
		if _, err := r.Resolve(context.Background()); err == nil || !strings.Contains(err.Error(), "E645") {
			t.Errorf("override %q accepted: %v", bad, err)
		}
	}
	good := "docker.io/library/alpine:3.22@" + pinnedDigest
	r := &Resolver{Getenv: envOf(map[string]string{EnvImage: good})}
	if ref, err := r.Resolve(context.Background()); err != nil || ref != good {
		t.Fatalf("ref %q err %v", ref, err)
	}
}

func TestFixtureTableRowResolves(t *testing.T) {
	r := &Resolver{
		Getenv:   envOf(map[string]string{EnvVersions: "/fixture/versions.yaml"}),
		ReadFile: func(p string) ([]byte, error) { return []byte(pinnedTable()), nil },
	}
	ref, err := r.Resolve(context.Background())
	if err != nil || ref != "docker.io/nself/ci-toolchain@"+pinnedDigest {
		t.Fatalf("ref %q err %v", ref, err)
	}
	r.ReadFile = func(string) ([]byte, error) { return nil, errors.New("gone") }
	if _, err := r.Resolve(context.Background()); err == nil || !strings.Contains(err.Error(), "E645") {
		t.Fatalf("unreadable table: %v", err)
	}
}

// fakeDocker records calls and answers inspect and pull from fixed results.
type fakeDocker struct {
	inspectRC, pullRC int
	calls             []string
}

func (f *fakeDocker) capture(_ context.Context, argv ...string) (string, int, error) {
	f.calls = append(f.calls, strings.Join(argv[:3], " "))
	switch argv[1] {
	case "image":
		return "", f.inspectRC, nil
	case "pull":
		if f.pullRC != 0 {
			return "manifest for the digest not found", f.pullRC, nil
		}
		return "pulled", 0, nil
	}
	return "", 1, nil
}

func TestEnsurePullsOnceAndMapsFailureToE645(t *testing.T) {
	ref := "docker.io/library/alpine:3.22@" + pinnedDigest
	env := envOf(map[string]string{EnvImage: ref})

	f := &fakeDocker{inspectRC: 1}
	r := &Resolver{Getenv: env, Capture: f.capture}
	for i := 0; i < 3; i++ {
		if got, err := r.Resolve(context.Background()); err != nil || got != ref {
			t.Fatalf("resolve %d: %q %v", i, got, err)
		}
	}
	if len(f.calls) != 2 || !strings.HasPrefix(f.calls[1], "docker pull") {
		t.Fatalf("calls %v, want one inspect and one pull", f.calls)
	}

	present := &fakeDocker{inspectRC: 0}
	if _, err := (&Resolver{Getenv: env, Capture: present.capture}).Resolve(context.Background()); err != nil || len(present.calls) != 1 {
		t.Fatalf("present image: %v calls %v", err, present.calls)
	}

	bad := &fakeDocker{inspectRC: 1, pullRC: 1}
	_, err := (&Resolver{Getenv: env, Capture: bad.capture}).Resolve(context.Background())
	if err == nil || !strings.Contains(err.Error(), "E645") || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("failed pull error = %v", err)
	}
}

// D-1185: with no docker CLI the image is inspected and pulled with podman.
func TestEnsureFallsBackToPodmanWhenDockerCannotRun(t *testing.T) {
	ref := "docker.io/library/alpine:3.22@" + pinnedDigest
	env := envOf(map[string]string{EnvImage: ref})
	var calls []string
	capture := func(_ context.Context, argv ...string) (string, int, error) {
		calls = append(calls, argv[0]+" "+argv[1])
		if argv[0] == "docker" { // the exec funnel reports a missing CLI as the shell's 127
			return "ci-job: line 0: exec: docker: not found", 127, nil
		}
		if argv[1] == "image" {
			return "", 1, nil
		}
		return "pulled", 0, nil
	}
	if got, err := (&Resolver{Getenv: env, Capture: capture}).Resolve(context.Background()); err != nil || got != ref {
		t.Fatalf("resolve: %q %v", got, err)
	}
	if want := "docker image,podman image,podman pull"; strings.Join(calls, ",") != want {
		t.Fatalf("calls %v, want %s", calls, want)
	}

	calls = nil
	present := func(_ context.Context, argv ...string) (string, int, error) {
		calls = append(calls, argv[0]+" "+argv[1])
		if argv[0] == "docker" {
			return "", -1, errors.New("executable file not found")
		}
		return "", 0, nil
	}
	if _, err := (&Resolver{Getenv: env, Capture: present}).Resolve(context.Background()); err != nil || len(calls) != 2 {
		t.Fatalf("image present under podman: %v calls %v", err, calls)
	}
}
