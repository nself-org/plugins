// Package tests holds the fragment policy test: it parses the plugin's compose
// fragment and static Traefik configuration and enforces ADR 0027 (no Docker
// provider, no socket, no ACME resolver, ssl/ read-only, one-shot renderer first).
package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/plugins/free/traefik/config"
	"github.com/nself-org/plugins/free/traefik/internal/staticcfg"
)

func readYAML(t *testing.T, rel string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", rel))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	return m
}

func get(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func TestFragmentPolicy(t *testing.T) {
	frag := readYAML(t, "docker-compose.plugin.yml")
	services := get(frag, "services").(map[string]any)
	for _, want := range []string{"traefik-config", "traefik-acme", "traefik"} {
		if _, ok := services[want]; !ok {
			t.Fatalf("service %s missing", want)
		}
	}
	raw, _ := os.ReadFile(filepath.Join("..", "docker-compose.plugin.yml"))
	for _, bad := range []string{"docker.sock", "DOCKER_HOST", "/var/run"} {
		if strings.Contains(string(raw), bad) {
			t.Errorf("fragment mentions %q", bad)
		}
	}
	for name, s := range services {
		svc := s.(map[string]any)
		if env, ok := svc["environment"]; ok && strings.Contains(strings.ToLower(toString(env)), "docker_host") {
			t.Errorf("%s sets DOCKER_HOST", name)
		}
		for _, v := range asList(svc["volumes"]) {
			if strings.Contains(toString(v), "docker.sock") {
				t.Errorf("%s mounts the Docker socket", name)
			}
			if strings.Contains(toString(v), "/nself/ssl") && !strings.HasSuffix(toString(v), ":ro") {
				t.Errorf("%s mounts ssl/ writable: %v", name, v)
			}
		}
	}
	tr := services["traefik"].(map[string]any)
	if !regexp.MustCompile(`^traefik:v3\.[0-9]+\.[0-9]+@sha256:[0-9a-f]{64}$`).MatchString(tr["image"].(string)) {
		t.Errorf("traefik image is not pinned by tag and digest: %v", tr["image"])
	}
	if got := get(tr, "depends_on", "traefik-config", "condition"); got != "service_completed_successfully" {
		t.Errorf("traefik must wait for traefik-config to complete, got %v", got)
	}
	if !hasSSLRO(tr) {
		t.Error("traefik does not mount ssl/ read-only")
	}
	if tr["read_only"] == true && !strings.Contains(toString(tr["tmpfs"]), "/tmp") {
		t.Error("traefik runs read-only without a tmpfs on /tmp: request bodies over the buffering memory limit would fail")
	}
	cfg := services["traefik-config"].(map[string]any)
	if !hasSSLRO(cfg) || cfg["restart"] != "no" {
		t.Error("traefik-config must mount ssl/ read-only and run once (restart no)")
	}
	if !strings.Contains(toString(get(cfg, "build", "context")), "${NSELF_PLUGIN_DIR}/traefik") {
		t.Error("build context is not anchored to the plugin directory")
	}
}

func TestStaticConfigPolicy(t *testing.T) {
	enabled, err := staticcfg.Render(config.Base, staticcfg.Upload{Enabled: true, ReadTimeout: 600 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name   string
		st     map[string]any
		upload bool
	}{
		{"config/traefik.yml", readYAML(t, "config/traefik.yml"), false},
		{"rendered static (upload enabled)", parseYAML(t, enabled), true},
	} {
		rel, st := c.name, c.st
		prov, _ := get(st, "providers").(map[string]any)
		if _, ok := prov["docker"]; ok || len(prov) != 1 || get(st, "providers", "file", "directory") != "/dynamic" || get(st, "providers", "file", "watch") != true {
			t.Errorf("%s: providers must be the file directory provider only: %v", rel, prov)
		}
		for _, ep := range []string{"web", "websecure"} {
			if v, ok := get(st, "entryPoints", ep, "http", "encodedCharacters", "allowEncodedSlash").(bool); !ok || v {
				t.Errorf("%s: entryPoints.%s must set http.encodedCharacters.allowEncodedSlash: false (blocked-path bypass via %%2F)", rel, ep)
			}
			// nginx client_header_timeout 60s; 0s (no deadline) leaves slowloris connections open forever.
			for key, want := range map[string]string{"readTimeout": "60s", "writeTimeout": "0s", "idleTimeout": "65s"} {
				if got := get(st, "entryPoints", ep, "transport", "respondingTimeouts", key); got != want {
					t.Errorf("%s: entryPoints.%s %s must be %s (nginx client_header_timeout 60s, keepalive_timeout 65, no total write cap), got %v", rel, ep, key, want, got)
				}
			}
		}
		up := get(st, "entryPoints", "upload")
		if (up != nil) != c.upload {
			t.Errorf("%s: upload entrypoint present=%v, want %v (only when upload routes are named)", rel, up != nil, c.upload)
		}
		if c.upload {
			if v, ok := get(st, "entryPoints", "upload", "http", "encodedCharacters", "allowEncodedSlash").(bool); !ok || v {
				t.Errorf("%s: entryPoints.upload must set allowEncodedSlash: false", rel)
			}
			if got := get(st, "entryPoints", "upload", "transport", "respondingTimeouts", "writeTimeout"); got != "0s" {
				t.Errorf("%s: entryPoints.upload writeTimeout must be 0s, got %v", rel, got)
			}
		}
		if get(st, "certificatesResolvers") != nil {
			t.Errorf("%s: certificatesResolvers present (certificates come from ssl/)", rel)
		}
		if get(st, "api") != nil {
			t.Errorf("%s: the shipped static config exposes the API (test-only)", rel)
		}
	}
}

// The image runs as the distroless nonroot uid; the long-running traefik-acme takes that default,
// the one-shot traefik-config is the one documented root exception, and traefik reads the rendered
// static directory read-only. The upload port stays on loopback unless the operator publishes it.
func TestNonRootAndUploadFragmentPolicy(t *testing.T) {
	df, _ := os.ReadFile(filepath.Join("..", "Dockerfile"))
	if !regexp.MustCompile(`(?m)^USER 65532:65532$`).Match(df) {
		t.Error("Dockerfile does not set USER 65532:65532")
	}
	services := get(readYAML(t, "docker-compose.plugin.yml"), "services").(map[string]any)
	if u, ok := services["traefik-acme"].(map[string]any)["user"]; ok {
		t.Errorf("traefik-acme sets user %v: it must use the image's non-root user", u)
	}
	if u := services["traefik-config"].(map[string]any)["user"]; u != "0:0" {
		t.Errorf("traefik-config user = %v, want the documented 0:0 exception", u)
	}
	// One render-interface tag on both renderer services, never a floating one: a stale local image
	// without --static-out would otherwise be reused after an upgrade.
	cfgImg, _ := services["traefik-config"].(map[string]any)["image"].(string)
	acmeImg, _ := services["traefik-acme"].(map[string]any)["image"].(string)
	if cfgImg != acmeImg || !regexp.MustCompile(`^nself/nself-traefik-render:r[0-9]+$`).MatchString(cfgImg) {
		t.Errorf("traefik-config and traefik-acme must share a render-generation tag (:rN, not :local or :latest), got %q and %q", cfgImg, acmeImg)
	}
	for _, n := range []string{"traefik-config", "traefik-acme"} {
		svc := services[n].(map[string]any)
		if get(svc, "build", "dockerfile") != "Dockerfile" || !strings.Contains(toString(get(svc, "build", "context")), "${NSELF_PLUGIN_DIR}/traefik") {
			t.Errorf("%s must declare the anchored build (the cli rewrites a bare image: line into its own per-project build)", n)
		}
	}
	tr := services["traefik"].(map[string]any)
	mounted := false
	for _, v := range asList(tr["volumes"]) {
		if s := toString(v); strings.HasSuffix(s, "traefik-static:/etc/traefik:ro") {
			mounted = true
		}
		if strings.Contains(toString(v), "config/traefik.yml") {
			t.Errorf("traefik mounts the hand-written base: %v", v)
		}
	}
	if !mounted {
		t.Error("traefik must mount the rendered traefik-static directory read-only at /etc/traefik")
	}
	found := false
	for _, v := range asList(tr["ports"]) {
		if s := toString(v); strings.HasSuffix(s, ":8444") {
			found = true
			if !strings.Contains(s, "${TRAEFIK_UPLOAD_PUBLISH:-127.0.0.1:}") {
				t.Errorf("the upload port must default to loopback, got %s", s)
			}
		}
	}
	if !found {
		t.Error("traefik does not publish the upload port 8444")
	}
	cmd := toString(services["traefik-config"].(map[string]any)["command"])
	for _, want := range []string{"--static-out", "--upload-routes", "--upload-read-timeout"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("traefik-config command lacks %s", want)
		}
	}
}

func parseYAML(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func hasSSLRO(svc map[string]any) bool {
	for _, v := range asList(svc["volumes"]) {
		if s := toString(v); strings.Contains(s, "/nself/ssl") && strings.HasSuffix(s, ":ro") {
			return true
		}
	}
	return false
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func toString(v any) string {
	b, _ := yaml.Marshal(v)
	return strings.TrimSpace(string(b))
}
