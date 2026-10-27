package main

// Purpose: the config digest preparePlan hands the coordinator (D-1270): the
// attest digest of the effective config on every path, never a placeholder,
// and a digest failure is an error rather than a silent fallback.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

var hexDigest = regexp.MustCompile(`^(sha256:)?[0-9a-f]{64}$`)

func digestRepo(t *testing.T, yaml string) RunOptions {
	t.Helper()
	dir := t.TempDir()
	if yaml != "" {
		p := filepath.Join(dir, ".nself", "ci.yaml")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("NSELF_CI_BASE_SHA", "")
	bare.cfg, bare.base = "", ""
	return RunOptions{Root: dir}
}

func TestPreparePlanDigestWithConfig(t *testing.T) {
	_, a, _, err := preparePlan(context.Background(), digestRepo(t, ciYAML))
	if err != nil {
		t.Fatal(err)
	}
	_, b, _, err := preparePlan(context.Background(), digestRepo(t, strings.Replace(ciYAML, "unit", "other", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if !hexDigest.MatchString(a) || a == b {
		t.Fatalf("config digests %q and %q must be distinct attest digests", a, b)
	}
}

func TestPreparePlanDigestWithoutConfig(t *testing.T) {
	_, got, _, err := preparePlan(context.Background(), digestRepo(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	want, err := attest.ConfigDigest(model.PipelineConfig{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got == "builtin-defaults" || got != want {
		t.Fatalf("no-config digest = %q, want the attest digest %q", got, want)
	}
}

func TestPreparePlanDigestErrorIsReturned(t *testing.T) {
	boom := errors.New("digest boom")
	old := configDigest
	configDigest = func(model.PipelineConfig) (string, error) { return "", boom }
	defer func() { configDigest = old }()
	for _, yaml := range []string{ciYAML, ""} {
		if _, _, _, err := preparePlan(context.Background(), digestRepo(t, yaml)); !errors.Is(err, boom) {
			t.Fatalf("preparePlan err = %v, want the digest error", err)
		}
	}
}

// The digest preparePlan hands the coordinator is the one evidence.InputsOf
// records for the same tree, with and without a config.
func TestPreparePlanDigestMatchesInputsOf(t *testing.T) {
	for name, yaml := range map[string]string{"with ci.yaml": ciYAML, "without ci.yaml": ""} {
		t.Run(name, func(t *testing.T) {
			o := digestRepo(t, yaml)
			_, got, _, err := preparePlan(context.Background(), o)
			if err != nil {
				t.Fatal(err)
			}
			in, err := evidence.InputsOf(os.DirFS(o.Root))
			if err != nil {
				t.Fatal(err)
			}
			if got != in.Config {
				t.Fatalf("preparePlan digest = %q, InputsOf config = %q", got, in.Config)
			}
		})
	}
}
