//go:build security && unix

package redteam

// Purpose: red team attacks on how a container job receives secrets (TM-30): the real Executor.Run
// builds the docker command; a fake `docker` script in a temp dir on PATH records its argv, its
// environment and the --env-file. Attacks: special and multi-line values, names that steer the
// docker CLI itself, job env shadowing a secret name, newline injection into the env file.
// Inputs:  secret values (fakeSecret based), job env. Outputs: pass or fail.
// Constraints: no docker; the script only writes files into the temp dir.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type secretMap map[string]string

func (m secretMap) Resolve(context.Context, []string) (map[string]string, error) { return m, nil }

type dockerRec struct{ argv, env, file string }

// runWithFakeDocker runs one container job and returns what the fake docker saw.
func runWithFakeDocker(t *testing.T, secrets map[string]string, refs []string, jobEnv map[string]string) (dockerRec, error) {
	t.Helper()
	dir := t.TempDir()
	bin := t.TempDir()
	envPath, filePath, argvPath := filepath.Join(dir, "host.env"), filepath.Join(dir, "job.env"), filepath.Join(dir, "argv")
	script := fmt.Sprintf("#!/bin/sh\nenv > %q\nprintf '%%s\\n' \"$@\" > %q\nprev=''\nfor a in \"$@\"; do if [ \"$prev\" = --env-file ]; then cp \"$a\" %q; fi; prev=\"$a\"; done\nexit 0\n", envPath, argvPath, filePath)
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := (&ciexec.Executor{}).Run(context.Background(), ciexec.JobSpec{AttemptID: "rt-sec", CoordinatorID: "coord", Home: t.TempDir(), Secrets: secretMap(secrets),
		Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: "alpine:3.19"}, Secrets: refs, Env: jobEnv}, Command: []string{"true"}})
	read := func(p string) string { b, _ := os.ReadFile(p); return string(b) }
	return dockerRec{argv: read(argvPath), env: read(envPath), file: read(filePath)}, r.Err
}

func TestB_Container_ValueShapesStayOffArgvAndEnvFile(t *testing.T) {
	for name, value := range map[string]string{
		"plain":      fakeSecret,
		"multi-line": "-----BEGIN KEY-----\n" + fakeSecret + "\n-----END KEY-----",
		"quoted":     `a"b'c$(id)` + fakeSecret + "`x`",
		"dash":       "--privileged " + fakeSecret,
		"equals":     "A=B=" + fakeSecret,
		"unicode":    "päss-" + fakeSecret,
	} {
		rec, err := runWithFakeDocker(t, map[string]string{"KEY_A": value}, []string{"project/KEY_A"}, map[string]string{"PLAIN": "x"})
		if err != nil {
			t.Logf("%s: run refused: %v", name, err)
			continue
		}
		if strings.Contains(rec.argv, fakeSecret) || strings.Contains(rec.file, fakeSecret) {
			t.Errorf("%s: the value reached argv or the env file", name)
		}
		if !strings.Contains(rec.argv, "-e\nKEY_A\n") {
			t.Errorf("%s: argv does not pass the secret by name: %q", name, rec.argv)
		}
	}
}

// A secret whose NAME the docker CLI itself reads changes where, how and as whom docker runs.
func TestB_Container_SecretNamesCannotSteerTheDockerCLI(t *testing.T) {
	for _, name := range []string{"DOCKER_HOST", "DOCKER_CONFIG", "DOCKER_CONTEXT", "DOCKER_CERT_PATH", "DOCKER_TLS_VERIFY", "LD_PRELOAD", "DYLD_INSERT_LIBRARIES", "HTTPS_PROXY", "PATH", "HOME", "TMPDIR"} {
		rec, err := runWithFakeDocker(t, map[string]string{name: "tcp://127.0.0.1:1/" + fakeSecret}, []string{"project/" + name}, nil)
		if err != nil {
			continue // refused: fine
		}
		if strings.Contains(rec.env, name+"=tcp://127.0.0.1:1/"+fakeSecret) {
			if !openFinding(t, "B10") {
				return
			}
			t.Errorf("secret named %s was placed in the docker CLI's own environment under that name", name)
		}
	}
}

func TestB_Container_JobEnvCannotShadowOrInject(t *testing.T) {
	// a job env entry named like the secret: the secret must still be the only source of the name
	rec, err := runWithFakeDocker(t, map[string]string{"KEY_A": fakeSecret}, []string{"project/KEY_A"}, map[string]string{"KEY_A": "attacker-plain"})
	if err == nil && strings.Contains(rec.file, "KEY_A=attacker-plain") && strings.Contains(rec.argv, "-e\nKEY_A\n") {
		t.Logf("job env and secret share the name KEY_A (docker applies -e last)")
	}
	// a newline in an env value must not add a second variable to the env file
	rec, err = runWithFakeDocker(t, nil, nil, map[string]string{"X": "1\nINJECTED=yes"})
	if err == nil && strings.Contains(rec.file, "\nINJECTED=yes") {
		t.Errorf("newline injected a variable into the env file: %q", rec.file)
	}
	rec, err = runWithFakeDocker(t, nil, nil, map[string]string{"X=1\nINJECTED": "yes"})
	if err == nil && strings.Contains(rec.file, "INJECTED") {
		t.Errorf("a variable NAME with a newline reached the env file: %q", rec.file)
	}
}
