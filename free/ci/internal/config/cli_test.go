package config

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testDeps(root string, env map[string]string, ignored bool) Deps {
	code := 1
	if ignored {
		code = 0
	}
	return Deps{
		Getenv:  envOf(env),
		Getwd:   func() (string, error) { return root, nil },
		HomeDir: func() (string, error) { return "/home/test", nil },
		Run:     func(context.Context, []string) (int, error) { return code, nil },
	}
}

type envelope struct {
	SchemaVersion string          `json:"schema_version"`
	Command       string          `json:"command"`
	Data          json.RawMessage `json:"data"`
	Error         *struct {
		Code     string `json:"code"`
		ExitCode int    `json:"exit_code"`
	} `json:"error"`
	Meta *struct {
		Warnings []string `json:"warnings"`
	} `json:"meta"`
}

func writeConfig(t *testing.T, root, doc string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestShowJSONNamesEverySource(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, jobsDoc)
	var out, errOut bytes.Buffer
	d := testDeps(root, map[string]string{"NSELF_CI_TIMEOUT": "20m"}, false)
	if code := RunShow([]string{"--json", "--home", "/h"}, &out, &errOut, d); code != 0 {
		t.Fatalf("exit %d: %s %s", code, out.String(), errOut.String())
	}
	var env envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if env.SchemaVersion != "1" || env.Command != "ci config show" {
		t.Errorf("envelope: %+v", env)
	}
	var data struct {
		Version     int  `json:"version"`
		FilePresent bool `json:"file_present"`
		Effective   struct {
			Repo, Timeout, SkipStatus, Home Value
			Jobs                            map[string]JobValues
		} `json:"effective"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatal(err)
	}
	e := data.Effective
	if data.Version != 1 || !data.FilePresent || e.Home.Source != SourceFlag || e.Timeout.Source != SourceEnv || e.Repo.Source != SourceDefault || e.Jobs["a"].Required.Source != SourceFile {
		t.Errorf("sources: %+v", data)
	}
	// Every effective value carries a source.
	var raw map[string]any
	_ = json.Unmarshal(env.Data, &raw)
	var walk func(path string, v any)
	walk = func(path string, v any) {
		m, ok := v.(map[string]any)
		if !ok {
			return
		}
		if _, hasValue := m["value"]; hasValue {
			if m["source"] == nil || m["source"] == "" {
				t.Errorf("%s has no source", path)
			}
			return
		}
		for k, c := range m {
			walk(path+"/"+k, c)
		}
	}
	walk("effective", raw["effective"])
}

func TestShowWithoutFileUsesDefaults(t *testing.T) {
	var out bytes.Buffer
	if code := RunShow([]string{"--json"}, &out, &bytes.Buffer{}, testDeps(t.TempDir(), nil, false)); code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	var env envelope
	_ = json.Unmarshal(out.Bytes(), &env)
	if !strings.Contains(string(env.Data), `"version": 1`) || !strings.Contains(string(env.Data), `"file_present": false`) {
		t.Errorf("data: %s", env.Data)
	}
}

func TestShowErrors(t *testing.T) {
	t.Run("leftover .ci.yaml is E621", func(t *testing.T) {
		root := t.TempDir()
		_ = os.WriteFile(filepath.Join(root, ".ci.yaml"), []byte("plugin: p\n"), 0o644)
		var out, errOut bytes.Buffer
		if code := RunShow(nil, &out, &errOut, testDeps(root, nil, false)); code != 1 {
			t.Fatalf("exit %d", code)
		}
		if !strings.Contains(errOut.String(), "E621") || !strings.Contains(errOut.String(), "nself ci config migrate --write") {
			t.Errorf("stderr: %s", errOut.String())
		}
	})
	t.Run("invalid config is E620 in a JSON error envelope", func(t *testing.T) {
		root := t.TempDir()
		writeConfig(t, root, "version: 1\nbogus: true\n")
		var out bytes.Buffer
		if code := RunShow([]string{"--json"}, &out, &bytes.Buffer{}, testDeps(root, nil, false)); code != 1 {
			t.Fatalf("exit %d", code)
		}
		var env envelope
		if err := json.Unmarshal(out.Bytes(), &env); err != nil || env.Error == nil || env.Error.Code != "E620" || env.Error.ExitCode != 1 {
			t.Errorf("%v %s", err, out.String())
		}
	})
	t.Run("bad env value", func(t *testing.T) {
		var errOut bytes.Buffer
		if code := RunShow(nil, &bytes.Buffer{}, &errOut, testDeps(t.TempDir(), map[string]string{"NSELF_CI_TIMEOUT": "soon"}, false)); code != 1 {
			t.Fatalf("exit %d", code)
		}
	})
	t.Run("unknown flag", func(t *testing.T) {
		if code := RunShow([]string{"--nope"}, &bytes.Buffer{}, &bytes.Buffer{}, testDeps(t.TempDir(), nil, false)); code != 1 {
			t.Fatalf("exit %d", code)
		}
	})
}

func TestShowWarnsOnIgnoredAndEnvPresent(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, "version: 1\njobs:\n  a:\n    kind: test\n    run: [x]\n    when:\n      env_present: [TOKEN_REF]\n")
	var out bytes.Buffer
	if code := RunShow([]string{"--json"}, &out, &bytes.Buffer{}, testDeps(root, nil, true)); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var env envelope
	_ = json.Unmarshal(out.Bytes(), &env)
	if env.Meta == nil || len(env.Meta.Warnings) != 2 || !strings.HasPrefix(env.Meta.Warnings[0], "E620:") || !strings.HasPrefix(env.Meta.Warnings[1], "E623:") {
		t.Errorf("warnings: %+v", env.Meta)
	}
}

func TestMigrateCommand(t *testing.T) {
	root := copyFixture(t, filepath.Join("legacy", "gauth"))
	d := testDeps(root, nil, true)
	var out, errOut bytes.Buffer
	if code := RunMigrate(nil, &out, &errOut, d); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "+  gauth-integration:") || !strings.Contains(out.String(), "nothing written") {
		t.Errorf("dry run output: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, FileName)); err == nil {
		t.Fatal("dry run wrote the file")
	}
	if !strings.Contains(errOut.String(), "E623") || !strings.Contains(errOut.String(), "!.nself/ci.yaml") {
		t.Errorf("E623 fix text missing: %s", errOut.String())
	}
	out.Reset()
	if code := RunMigrate([]string{"--write", "--json"}, &out, &bytes.Buffer{}, d); code != 0 {
		t.Fatalf("write exit %d: %s", code, out.String())
	}
	var env envelope
	_ = json.Unmarshal(out.Bytes(), &env)
	if !strings.Contains(string(env.Data), `"written": true`) || env.Meta == nil || len(env.Meta.Warnings) != 1 {
		t.Errorf("write envelope: %s", out.String())
	}
	if _, err := Load(filepath.Join(root, FileName)); err != nil {
		t.Fatalf("written file does not load: %v", err)
	}
	// With both files present show refuses (E621) until the legacy file goes.
	if code := RunShow(nil, &bytes.Buffer{}, &bytes.Buffer{}, d); code != 1 {
		t.Errorf("show with a leftover .ci.yaml: exit %d", code)
	}
	if err := os.Remove(filepath.Join(root, "free", "plugin-gauth", ".ci.yaml")); err != nil {
		t.Fatal(err)
	}
	if code := RunShow(nil, &bytes.Buffer{}, &bytes.Buffer{}, d); code != 0 {
		t.Errorf("show after deleting the legacy file: exit %d", code)
	}
}

func TestMigrateCommandErrors(t *testing.T) {
	var plain bytes.Buffer
	if code := RunMigrate(nil, &plain, &bytes.Buffer{}, testDeps(t.TempDir(), nil, false)); code != 0 || !strings.Contains(plain.String(), "nothing to migrate") {
		t.Errorf("empty repo: exit %d %s", code, plain.String())
	}
	var empty bytes.Buffer
	if code := RunMigrate([]string{"--json"}, &empty, &bytes.Buffer{}, testDeps(t.TempDir(), nil, false)); code != 0 || !strings.Contains(empty.String(), `"jobs": []`) {
		t.Errorf("empty repo json: exit %d %s", code, empty.String())
	}
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, ".ci.yaml"), []byte("plugin: p\nmystery: 1\nstages: []\n"), 0o644)
	var out bytes.Buffer
	if code := RunMigrate([]string{"--json"}, &out, &bytes.Buffer{}, testDeps(root, nil, false)); code != 1 || !strings.Contains(out.String(), `"E622"`) {
		t.Errorf("E622: exit %d %s", code, out.String())
	}
}
