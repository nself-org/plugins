//go:build security

package redteam

// Purpose: TM-42 red team. The free/forgejo and free/github-runner plugin templates are parsed and
// attacked statically: docker-socket mounts, privileged or host-namespace settings, public port
// binds, default credentials, open registration, the public-repository opt-out default, image
// pinning, and whether the runner's control API is reachable without a credential.
// Inputs:  docker-compose.plugin.yml, Dockerfile and Go sources of the two plugins.
// Outputs: pass = the dangerous setting is absent (control HELD).
// Constraints: static reads only; nothing is started and no docker is used.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const dPluginRoot = "../../../../" // free/ci/test/security/redteam -> free/

func dRead(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dPluginRoot, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func dCompose(t *testing.T, rel string) map[string]map[string]any {
	t.Helper()
	var doc struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(dRead(t, rel)), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Services) == 0 {
		t.Fatalf("%s: no services parsed", rel)
	}
	return doc.Services
}

func dStrings(v any) []string {
	var out []string
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			out = append(out, dStrings(e)...)
		}
	case map[string]any:
		for k, e := range x {
			out = append(out, k)
			out = append(out, dStrings(e)...)
		}
	case string:
		out = append(out, x)
	}
	return out
}

// dDangerous lists the settings that hand a container the host.
func dDangerous(name string, svc map[string]any) []string {
	var bad []string
	for _, key := range []string{"privileged", "cap_add", "devices", "security_opt", "pid", "ipc", "userns_mode", "extra_hosts", "sysctls"} {
		if v, ok := svc[key]; ok && v != false {
			bad = append(bad, name+": "+key)
		}
	}
	if m, _ := svc["network_mode"].(string); m != "" {
		bad = append(bad, name+": network_mode "+m)
	}
	for _, v := range dStrings(svc["volumes"]) {
		l := strings.ToLower(v)
		if strings.Contains(l, "docker.sock") || strings.Contains(l, "podman.sock") || strings.HasPrefix(l, "/:") || strings.HasPrefix(l, "/var/run") || strings.HasPrefix(l, "/etc:") || strings.HasPrefix(l, "/root") {
			bad = append(bad, name+": host mount "+v)
		}
	}
	for _, p := range dStrings(svc["ports"]) {
		if !strings.HasPrefix(p, "127.0.0.1:") && !strings.HasPrefix(p, "[::1]:") {
			bad = append(bad, name+": port not loopback "+p)
		}
	}
	return bad
}

var dDigest = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)

// D20: forgejo keeps no runner, no socket, no privilege, loopback ports, a pinned image, closed
// sign-up and no default admin credential.
func TestD20_ForgejoTemplateHasNoHostHandles(t *testing.T) {
	if len(dDangerous("probe", map[string]any{"privileged": true, "volumes": []any{"/var/run/docker.sock:/var/run/docker.sock"}, "ports": []any{"0.0.0.0:80:80"}})) != 3 {
		t.Fatal("positive control: the detector missed a planted socket mount, privileged flag or public port")
	}
	svcs := dCompose(t, "forgejo/docker-compose.plugin.yml")
	for name, svc := range svcs {
		if bad := dDangerous(name, svc); len(bad) > 0 {
			t.Errorf("dangerous settings: %v", bad)
		}
		if img, _ := svc["image"].(string); !dDigest.MatchString(img) {
			t.Errorf("%s: image %q is not digest-pinned", name, img)
		}
		if strings.Contains(strings.ToLower(name), "runner") {
			t.Errorf("service %s: an in-stack runner returned", name)
		}
	}
	raw := dRead(t, "forgejo/docker-compose.plugin.yml")
	for _, want := range []string{`DISABLE_REGISTRATION: "${FORGEJO_DISABLE_REGISTRATION:-true}"`, `INSTALL_LOCK: "true"`} {
		if !strings.Contains(raw, want) {
			t.Errorf("forgejo template lost %s", want)
		}
	}
	for _, v := range []string{"NSELF_FORGEJO_ADMIN_PASSWORD", "NSELF_FORGEJO_ADMIN_USER"} {
		if regexp.MustCompile(v + `:\s*\$\{` + v + `:-`).MatchString(raw) {
			t.Errorf("%s has a default value in the template", v)
		}
	}
	if boot := dRead(t, "forgejo/forgejo-bootstrap.sh"); !strings.Contains(boot, `:?NSELF_FORGEJO_ADMIN_PASSWORD is required`) || regexp.MustCompile(`--password\s+["']?[A-Za-z0-9]`).MatchString(boot) {
		t.Error("bootstrap script can create the admin with a literal password")
	}
}

// D21: github-runner template has no host handles, keeps the public-repository opt-out off, takes no
// default PAT, and its image is pinned and not root.
func TestD21_GithubRunnerTemplateAndImage(t *testing.T) {
	svcs := dCompose(t, "github-runner/docker-compose.plugin.yml")
	raw := dRead(t, "github-runner/docker-compose.plugin.yml")
	for name, svc := range svcs {
		if bad := dDangerous(name, svc); len(bad) > 0 {
			t.Errorf("dangerous settings: %v", bad)
		}
	}
	if !strings.Contains(raw, "GITHUB_RUNNER_ALLOW_PUBLIC_REPOS: ${GITHUB_RUNNER_ALLOW_PUBLIC_REPOS:-false}") {
		t.Error("the public-repository opt-out no longer defaults to false")
	}
	for _, v := range []string{"GITHUB_RUNNER_PAT", "GITHUB_RUNNER_ORG"} {
		if strings.Contains(raw, v+": ${"+v+":-") {
			t.Errorf("%s has a default", v)
		}
	}
	if !openFinding(t, "D21") {
		return
	}
	for name, svc := range svcs {
		if img, _ := svc["image"].(string); !dDigest.MatchString(img) {
			t.Errorf("%s: image %q is a mutable tag", name, img)
		}
		if u, _ := svc["user"].(string); u == "" && !regexp.MustCompile(`(?m)^USER\s+[^r\s]`).MatchString(dRead(t, "github-runner/Dockerfile")) {
			t.Errorf("%s: the container runs as root (no user in compose, no USER in the Dockerfile)", name)
		}
	}
}

// D22: the compose file hands the plugin PLUGIN_INTERNAL_SECRET, so the control routes (register, start,
// stop, logs) must demand it. Any process on the project network can otherwise register, start or stop
// the runner on the deploy host and read its logs.
func TestD22_RunnerControlAPIDemandsTheInternalSecret(t *testing.T) {
	if !openFinding(t, "D22") {
		return
	}
	if !strings.Contains(dRead(t, "github-runner/docker-compose.plugin.yml"), "PLUGIN_INTERNAL_SECRET") {
		t.Fatal("template no longer passes PLUGIN_INTERNAL_SECRET (positive control)")
	}
	found := false
	_ = filepath.WalkDir(filepath.Join(dPluginRoot, "github-runner"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() && (d.Name() == "vendor" || d.Name() == "third_party" || d.Name() == "tests") {
			if err == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
			b, _ := os.ReadFile(p)
			if strings.Contains(string(b), "PLUGIN_INTERNAL_SECRET") {
				found = true
			}
		}
		return nil
	})
	if !found {
		t.Fatal("no Go source of the plugin reads PLUGIN_INTERNAL_SECRET: /v1/register, /v1/start, /v1/stop and /v1/logs are unauthenticated")
	}
}

// D23: the same routes are served with Access-Control-Allow-Origin: * (sdk.CORS), so a web page the
// operator opens can call the loopback port and read /v1/logs.
func TestD23_RunnerControlAPIIsNotCrossOriginOpen(t *testing.T) {
	if !openFinding(t, "D23") {
		return
	}
	main := dRead(t, "github-runner/cmd/main.go")
	if strings.Contains(main, "sdk.CORS") {
		t.Fatal("cmd/main.go wraps the unauthenticated control routes in sdk.CORS (Allow-Origin *)")
	}
}
