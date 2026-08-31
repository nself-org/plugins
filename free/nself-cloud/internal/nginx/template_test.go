package nginx_test

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins-pro/paid/nself-cloud/internal/nginx"
)

var validVars = nginx.ServerBlockVars{
	Hostname:     "app.mycorp.com",
	TenantID:     "550e8400-e29b-41d4-a716-446655440000",
	InstanceID:   "660e8400-e29b-41d4-a716-446655440001",
	CertPath:     "/etc/nginx/certs/app.mycorp.com/cert.pem",
	KeyPath:      "/etc/nginx/certs/app.mycorp.com/key.pem",
	UpstreamHost: "127.0.0.1:8080",
}

func TestGenerateConfig_ContainsTenantID(t *testing.T) {
	t.Parallel()
	out, err := nginx.GenerateConfig(validVars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content := string(out)
	if !strings.Contains(content, `set $tenant_id "550e8400-e29b-41d4-a716-446655440000"`) {
		t.Errorf("config missing tenant_id set directive\ngot:\n%s", content)
	}
}

func TestGenerateConfig_ContainsHostname(t *testing.T) {
	t.Parallel()
	out, err := nginx.GenerateConfig(validVars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "server_name app.mycorp.com") {
		t.Errorf("config missing server_name directive")
	}
}

func TestGenerateConfig_ContainsTLSPaths(t *testing.T) {
	t.Parallel()
	out, err := nginx.GenerateConfig(validVars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content := string(out)
	if !strings.Contains(content, "ssl_certificate ") {
		t.Errorf("config missing ssl_certificate directive")
	}
	if !strings.Contains(content, "ssl_certificate_key ") {
		t.Errorf("config missing ssl_certificate_key directive")
	}
}

func TestGenerateConfig_ContainsHTTPSRedirect(t *testing.T) {
	t.Parallel()
	out, err := nginx.GenerateConfig(validVars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "return 301 https://") {
		t.Errorf("config missing HTTP→HTTPS redirect")
	}
}

func TestGenerateConfig_ContainsXTenantHeader(t *testing.T) {
	t.Parallel()
	out, err := nginx.GenerateConfig(validVars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "X-Tenant-ID") {
		t.Errorf("config missing X-Tenant-ID proxy header")
	}
}

func TestGenerateConfig_MissingFields(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		vars nginx.ServerBlockVars
	}{
		{"no hostname", nginx.ServerBlockVars{TenantID: "550e8400-e29b-41d4-a716-446655440000", InstanceID: "660e8400-e29b-41d4-a716-446655440001", CertPath: "/c", KeyPath: "/k", UpstreamHost: "127.0.0.1:8080"}},
		{"no tenant", nginx.ServerBlockVars{Hostname: "a.com", InstanceID: "660e8400-e29b-41d4-a716-446655440001", CertPath: "/c", KeyPath: "/k", UpstreamHost: "127.0.0.1:8080"}},
		{"invalid tenant uuid", nginx.ServerBlockVars{Hostname: "a.com", TenantID: "not-a-uuid", InstanceID: "660e8400-e29b-41d4-a716-446655440001", CertPath: "/c", KeyPath: "/k", UpstreamHost: "127.0.0.1:8080"}},
		{"no upstream", nginx.ServerBlockVars{Hostname: "a.com", TenantID: "550e8400-e29b-41d4-a716-446655440000", InstanceID: "660e8400-e29b-41d4-a716-446655440001", CertPath: "/c", KeyPath: "/k"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := nginx.GenerateConfig(tc.vars); err == nil {
				t.Errorf("expected error for %q, got nil", tc.name)
			}
		})
	}
}

func TestGenerateConfig_InjectionRejected(t *testing.T) {
	t.Parallel()
	// Newline injection in hostname field should be rejected.
	vars := validVars
	vars.Hostname = "evil.com\nset $injected 1"
	if _, err := nginx.GenerateConfig(vars); err == nil {
		t.Error("expected error for newline-injected hostname, got nil")
	}
}
