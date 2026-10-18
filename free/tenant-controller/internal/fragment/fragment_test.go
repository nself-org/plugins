// Package fragment holds checks on docker-compose.plugin.yml, which no Go code reads.
package fragment

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func load(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../docker-compose.plugin.yml")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// service returns the text of one top-level service block.
func service(t *testing.T, src, name string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(name) + `:\n((?:    .*\n|\n)*)`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("service %s missing", name)
	}
	return m[1]
}

func TestControllerSharesNginxConfDir(t *testing.T) {
	s := service(t, load(t), "plugin-tenant-controller")
	if !strings.Contains(s, "./nginx/conf.d:/etc/nginx/conf.d") {
		t.Fatal("controller must mount the stack nginx conf.d so the vhost file reaches nginx")
	}
	if !strings.Contains(s, "NGINX_RELOAD_MODE: watch") {
		t.Fatal("the image has no nginx binary: reload mode must be watch")
	}
	if strings.Contains(s, "docker.sock") {
		t.Fatal("no Docker socket")
	}
}

func TestReloaderSidecarSignalsNginxWithoutDockerSocket(t *testing.T) {
	s := service(t, load(t), "plugin-tenant-controller-reloader")
	for _, want := range []string{`pid: "service:nginx"`, "./nginx/conf.d:/etc/nginx/conf.d:ro", "kill -HUP"} {
		if !strings.Contains(s, want) {
			t.Errorf("reloader missing %q", want)
		}
	}
	if strings.Contains(s, "docker.sock") {
		t.Fatal("no Docker socket")
	}
}

func TestDisabledControllerDoesNotRestartLoop(t *testing.T) {
	src := load(t)
	s := service(t, src, "plugin-tenant-controller")
	if !strings.Contains(s, "NSELF_CONTROLLER_ENABLED: ${NSELF_CONTROLLER_ENABLED:-false}") {
		t.Fatal("must default to disabled")
	}
	if strings.Contains(s, "restart: unless-stopped") || !strings.Contains(s, "restart: on-failure") {
		t.Fatal("a disabled controller exits 0; unless-stopped would restart it forever")
	}
	// Required vars must not be hard-required by compose interpolation either.
	if strings.Contains(s, "${NSELF_CONTROLLER_ADMIN_TOKEN}") || strings.Contains(s, "${NSELF_CONTROLLER_BASE_DOMAIN}") {
		t.Fatal("optional-at-rest vars need :- defaults so compose does not warn while disabled")
	}
}
