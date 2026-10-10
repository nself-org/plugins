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

	"gopkg.in/yaml.v3"
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
	cfg := services["traefik-config"].(map[string]any)
	if !hasSSLRO(cfg) || cfg["restart"] != "no" {
		t.Error("traefik-config must mount ssl/ read-only and run once (restart no)")
	}
	if !strings.Contains(toString(get(cfg, "build", "context")), "${NSELF_PLUGIN_DIR}/traefik") {
		t.Error("build context is not anchored to the plugin directory")
	}
}

func TestStaticConfigPolicy(t *testing.T) {
	for _, rel := range []string{"config/traefik.yml", "tests/traefik-test.yml"} {
		st := readYAML(t, rel)
		prov, _ := get(st, "providers").(map[string]any)
		if _, ok := prov["docker"]; ok || len(prov) != 1 || get(st, "providers", "file", "directory") != "/dynamic" || get(st, "providers", "file", "watch") != true {
			t.Errorf("%s: providers must be the file directory provider only: %v", rel, prov)
		}
		if get(st, "certificatesResolvers") != nil {
			t.Errorf("%s: certificatesResolvers present (certificates come from ssl/)", rel)
		}
	}
	if get(readYAML(t, "config/traefik.yml"), "api") != nil {
		t.Error("the shipped static config exposes the API (test-only)")
	}
	if get(readYAML(t, "tests/traefik-test.yml"), "api", "insecure") != true {
		t.Error("the test static config must enable api.insecure")
	}
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
