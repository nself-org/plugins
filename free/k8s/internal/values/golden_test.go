// Purpose: golden and invariant tests for the mapper over the two fixtures:
// full (cli `nself build` output) and interp (${VAR:-default} plus a plugin
// env file, resolved by real compose).
//
// Inputs: testdata/full, testdata/interp.
//
// Outputs: test results.
//
// Constraints: the same checks run on both fixtures.
package values

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	fixtures  = []string{"full", "interp"}
	secretKey = regexp.MustCompile(`SECRET|PASSWORD|TOKEN|KEY|DATABASE_URL`)
)

func TestGoldenFiles(t *testing.T) {
	for _, name := range fixtures {
		_, v, s := mapFixture(t, name)
		vb, err := MarshalValues(v)
		if err != nil {
			t.Fatal(err)
		}
		sb, err := MarshalSecrets(s)
		if err != nil {
			t.Fatal(err)
		}
		compareGolden(t, name, "values.yaml", vb)
		compareGolden(t, name, "secrets.yaml", sb)
	}
}

func TestEveryServiceMappedOrUnsupported(t *testing.T) {
	for _, name := range fixtures {
		m, v, _ := mapFixture(t, name)
		for svc := range m.Services {
			_, mapped := v.Services[svc]
			if mapped == hasUnsupported(v.Unsupported, svc) {
				t.Errorf("%s: service %s is mapped=%v unsupported=%v (want exactly one)", name, svc, mapped, !mapped)
			}
		}
		for _, u := range v.Unsupported {
			if u.Reason == "" {
				t.Errorf("%s: unsupported %s has no reason", name, u.Name)
			}
		}
	}
	_, v, _ := mapFixture(t, "full")
	for _, want := range []string{"worker-one", "worker-two"} {
		if !hasUnsupported(v.Unsupported, want) {
			t.Errorf("build-context service %s should be unsupported", want)
		}
	}
	if k := v.Services["postgres"].Kind; k != KindStatefulSet {
		t.Errorf("postgres kind = %q, want statefulset", k)
	}
	if k := v.Services["meilisearch-init"].Kind; k != KindJob {
		t.Errorf("meilisearch-init kind = %q, want job", k)
	}
	if k := v.Services["nginx"].Kind; k != KindIngress {
		t.Errorf("nginx kind = %q, want ingress", k)
	}
}

func TestSecretsHoldEveryEnvValueValuesHoldNone(t *testing.T) {
	for _, name := range fixtures {
		m, v, s := mapFixture(t, name)
		vb, _ := MarshalValues(v)
		for svc, cs := range m.Services {
			if _, ok := v.Services[svc]; !ok || v.Services[svc].Kind == KindIngress {
				continue
			}
			for k, val := range cs.Environment {
				if val == nil {
					continue
				}
				if got := s.Secrets[svc][k]; got != *val {
					t.Errorf("%s: secrets[%s][%s] = %q, want %q", name, svc, k, got, *val)
				}
				// A healthcheck command may legitimately repeat a plain value
				// (a URL); credential-bearing keys must never reach values.yaml.
				if secretKey.MatchString(k) && len(*val) >= 12 && strings.Contains(string(vb), *val) {
					t.Errorf("%s: values.yaml contains the env value of %s.%s", name, svc, k)
				}
			}
		}
	}
}

func TestNoInterpolationSurvives(t *testing.T) {
	for _, name := range fixtures {
		_, v, s := mapFixture(t, name)
		vb, _ := MarshalValues(v)
		sb, _ := MarshalSecrets(s)
		if strings.Contains(string(vb), "${") || strings.Contains(string(sb), "${") {
			t.Errorf("%s: an unresolved ${ reached values or secrets", name)
		}
	}
	_, v, s := mapFixture(t, "interp")
	if got := v.Services["api"]; got.Image != "hasura/graphql-engine" || got.Tag != "v2.50.1" {
		t.Errorf("api image = %s:%s, want the env-file tag v2.50.1", got.Image, got.Tag)
	}
	if got := v.Services["db"].Tag; got != "16-alpine" {
		t.Errorf("db tag = %q, want the ${PG_TAG:-16-alpine} default", got)
	}
	if got := v.Services["demo"]; got.Image != "ghcr.io/example/demo" || got.Tag != "1.2.3" {
		t.Errorf("demo image = %s:%s, want plugin env file tag 1.2.3", got.Image, got.Tag)
	}
	if got := s.Secrets["demo"]["DEMO_TOKEN"]; got != "fixture-demo-token" {
		t.Errorf("demo token = %q, want the plugin env file value", got)
	}
}

func TestUnresolvedInterpolationIsRejected(t *testing.T) {
	m := &Model{Name: "x", Services: map[string]ComposeSvc{
		"web": {Image: "nginx:1", Environment: map[string]*string{"A": ptr("${NOPE}")}},
	}}
	_, _, err := Map(m, nil)
	if err == nil || !strings.Contains(err.Error(), "web") || strings.Contains(err.Error(), "NOPE}") {
		t.Fatalf("want an error naming the service but not the value, got %v", err)
	}
}

// TestRealComposeMatchesRecording runs the real compose binary over the
// interp fixture and requires the same values as the recorded JSON gives.
func TestRealComposeMatchesRecording(t *testing.T) {
	if !composeAvailable() {
		t.Skip("no docker compose available")
	}
	dir, err := filepath.Abs(fixtureDir("interp"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := Resolve(context.Background(), dir, ExecRunner)
	if err != nil {
		t.Fatal(err)
	}
	v, s, err := Map(m, nil)
	if err != nil {
		t.Fatal(err)
	}
	vb, _ := MarshalValues(v)
	sb, _ := MarshalSecrets(s)
	for file, got := range map[string][]byte{"values.yaml": vb, "secrets.yaml": sb} {
		want, err := os.ReadFile(filepath.Join(fixtureDir("interp"), "golden", file))
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != string(got) {
			t.Errorf("real compose %s differs from golden:\n%s", file, got)
		}
	}
}

func composeAvailable() bool {
	if err := exec.Command("docker", "compose", "version").Run(); err == nil {
		return true
	}
	_, err := exec.LookPath("docker-compose")
	return err == nil
}

func ptr(s string) *string { return &s }
