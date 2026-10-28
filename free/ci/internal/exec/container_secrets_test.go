package exec

// A secret reaches a baseline container only as `-e NAME` plus the docker CLI's own environment:
// never in argv, never in the --env-file (P7-TRUST-02).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type fixedSecrets map[string]string

func (f fixedSecrets) Resolve(context.Context, []string) (map[string]string, error) { return f, nil }

func TestContainerSecretIsEnvOnly(t *testing.T) {
	const value = "container-fake-value-61e0aa"
	dir := t.TempDir()
	envPath, filePath, argvPath := filepath.Join(dir, "host.env"), filepath.Join(dir, "job.env"), filepath.Join(dir, "argv")
	fakeDocker(t, fmt.Sprintf("env > %q\nprintf '%%s\\n' \"$@\" > %q\nfor a in \"$@\"; do if [ \"$prev\" = --env-file ]; then cp \"$a\" %q; fi; prev=\"$a\"; done\nexit 0\n", envPath, argvPath, filePath))
	r := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), JobSpec{AttemptID: "sec", CoordinatorID: "coord", Home: t.TempDir(), Secrets: fixedSecrets{"FOO": value},
		Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: "alpine:3.19"}, Secrets: []string{"project/FOO"}, Env: map[string]string{"PLAIN": "x"}}, Command: []string{"true"}})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	read := func(p string) string {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	argv, file, host := read(argvPath), read(filePath), read(envPath)
	if strings.Contains(argv, value) || strings.Contains(file, value) || !strings.Contains(host, "FOO="+value) {
		t.Fatalf("value placement wrong:\nargv=%q\nenvfile=%q", argv, file)
	}
	if !strings.Contains(argv, "-e\nFOO\n") || !strings.Contains(file, "PLAIN=x") {
		t.Fatalf("argv %q envfile %q", argv, file)
	}
}
