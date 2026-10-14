package config

import "testing"

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func loaded(t *testing.T, doc string) *Loaded {
	t.Helper()
	l, err := Parse("ci.yaml", []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

const jobsDoc = "version: 1\njobs:\n  a:\n    kind: test\n    run: [x]\n    timeout: 90s\n    required: false\n    path: fast\n    retry: {infra_max: 2}\n  b:\n    kind: test\n    run: [y]\n"

func TestPrecedence(t *testing.T) {
	defaults := Defaults{Repo: "/cwd", Home: "/home/.nself/ci"}
	// Detected default only.
	e, err := Resolve(Inputs{Defaults: defaults})
	if err != nil {
		t.Fatal(err)
	}
	if e.Repo != (Value{"/cwd", SourceDefault, "detected"}) || e.Timeout.Value != DefaultTimeout || e.SkipStatus.Value != false {
		t.Errorf("defaults: %+v", e)
	}
	// Env beats default.
	e, _ = Resolve(Inputs{Defaults: defaults, Env: envOf(map[string]string{"NSELF_CI_REPO": "/env", "NSELF_CI_SKIP_STATUS": "1", "NSELF_CI_HOME": "/envhome"})})
	if e.Repo.Source != SourceEnv || e.Repo.Origin != "NSELF_CI_REPO" || e.SkipStatus.Value != true || e.Home.Value != "/envhome" {
		t.Errorf("env: %+v", e)
	}
	// Flag beats env.
	e, _ = Resolve(Inputs{Defaults: defaults, Flags: map[string]string{"repo": "/flag"}, Env: envOf(map[string]string{"NSELF_CI_REPO": "/env"})})
	if e.Repo != (Value{"/flag", SourceFlag, "--repo"}) {
		t.Errorf("flag: %+v", e.Repo)
	}
	// File values, then env over file, then flag over env, per job.
	l := loaded(t, jobsDoc)
	e, _ = Resolve(Inputs{Defaults: defaults, Loaded: l})
	a, b := e.Jobs["a"], e.Jobs["b"]
	if a.Timeout != (Value{"90s", SourceFile, "ci.yaml:6"}) || a.Required != (Value{false, SourceFile, "ci.yaml:7"}) || a.Path.Value != "fast" || a.RetryInfraMax.Value != 2 {
		t.Errorf("file values: %+v", a)
	}
	if b.Timeout.Source != SourceDefault || b.Required.Value != true || b.Path.Value != "deep" || b.RetryInfraMax.Value != 1 {
		t.Errorf("job defaults: %+v", b)
	}
	e, _ = Resolve(Inputs{Defaults: defaults, Loaded: l, Env: envOf(map[string]string{"NSELF_CI_TIMEOUT": "30m"})})
	if e.Jobs["a"].Timeout != (Value{"30m", SourceEnv, "NSELF_CI_TIMEOUT"}) || e.Timeout.Source != SourceEnv {
		t.Errorf("env over file: %+v", e.Jobs["a"].Timeout)
	}
	e, _ = Resolve(Inputs{Defaults: defaults, Loaded: l, Flags: map[string]string{"timeout": "5m"}, Env: envOf(map[string]string{"NSELF_CI_TIMEOUT": "30m"})})
	if e.Jobs["a"].Timeout != (Value{"5m", SourceFlag, "--timeout"}) {
		t.Errorf("flag over env: %+v", e.Jobs["a"].Timeout)
	}
}

func TestPrecedenceRejectsBadValues(t *testing.T) {
	for name, in := range map[string]Inputs{
		"timeout env":    {Env: envOf(map[string]string{"NSELF_CI_TIMEOUT": "soon"})},
		"timeout zero":   {Flags: map[string]string{"timeout": "0s"}},
		"skip status":    {Env: envOf(map[string]string{"NSELF_CI_SKIP_STATUS": "maybe"})},
		"flag no-status": {Flags: map[string]string{"no-status": "perhaps"}},
	} {
		_, err := Resolve(in)
		e, ok := err.(*Error)
		if !ok || e.Code != CodeInvalid || e.Problems[0].Pointer == "" {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestBindingsDeclareEachEnvOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range Bindings {
		if seen[b.Env] || seen[b.Name] || b.Env == "NSELF_CI_SHA" {
			t.Errorf("binding %+v duplicated or v1.4-only", b)
		}
		seen[b.Env], seen[b.Name] = true, true
	}
	for _, want := range []string{"NSELF_CI_REPO", "NSELF_CI_TIMEOUT", "NSELF_CI_SKIP_STATUS", "NSELF_CI_HOME"} {
		if !seen[want] {
			t.Errorf("missing binding %s", want)
		}
	}
}
