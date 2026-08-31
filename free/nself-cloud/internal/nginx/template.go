// Package nginx generates and installs per-domain nginx server block configurations
// for nself-cloud tenant instances. Each generated config includes a
// `set $tenant_id "<uuid>";` directive for downstream logging and routing.
package nginx

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"
)

const serverBlockTpl = `# nself-cloud: auto-generated — DO NOT HAND EDIT
# Generated: {{ .GeneratedAt }}
# Tenant:    {{ .TenantID }}
# Instance:  {{ .InstanceID }}
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name {{ .Hostname }};

    # Tenant label for access-log enrichment and upstream routing.
    set $tenant_id "{{ .TenantID }}";

    # TLS — cert and key managed by nself-cloud Let's Encrypt automation.
    ssl_certificate     {{ .CertPath }};
    ssl_certificate_key {{ .KeyPath }};
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers off;
    ssl_session_cache   shared:SSL:10m;
    ssl_session_timeout 1d;

    # HSTS
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;

    # Proxy to the tenant's nSelf instance (Hasura + Auth on standard ports).
    location / {
        proxy_pass         http://{{ .UpstreamHost }};
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
        proxy_set_header   X-Tenant-ID       $tenant_id;
        proxy_read_timeout 60s;
    }
}

# HTTP → HTTPS redirect.
server {
    listen 80;
    listen [::]:80;
    server_name {{ .Hostname }};
    return 301 https://$host$request_uri;
}
`

// ServerBlockVars holds the template variables for a single tenant domain nginx config.
type ServerBlockVars struct {
	// Hostname is the tenant's custom domain (e.g. "app.mycorp.com").
	Hostname string
	// TenantID is the UUID from np_cloud_tenants.id.
	TenantID string
	// InstanceID is the UUID from np_cloud_instances.id.
	InstanceID string
	// CertPath is the local path to the Let's Encrypt cert.pem (or bundle.pem).
	CertPath string
	// KeyPath is the local path to the private key.pem.
	KeyPath string
	// UpstreamHost is the internal address of the tenant's nSelf backend
	// (e.g. "127.0.0.1:8080" or a UNIX socket path without the unix: prefix).
	UpstreamHost string
	// GeneratedAt is the timestamp embedded in the config comment.
	GeneratedAt string
}

// Generator creates and installs nginx server block configs.
type Generator struct {
	// SitesEnabledDir is the directory where config files are written.
	// Defaults to /etc/nginx/sites-enabled on production Hetzner servers.
	SitesEnabledDir string
	// ReloadCommand is the command used to reload nginx after writing a config.
	// Defaults to []string{"nginx", "-s", "reload"}.
	ReloadCommand []string
}

// DefaultSitesEnabledDir is the standard path on Hetzner Ubuntu servers.
const DefaultSitesEnabledDir = "/etc/nginx/sites-enabled"

// NewGenerator returns a Generator with production defaults.
func NewGenerator() *Generator {
	return &Generator{
		SitesEnabledDir: DefaultSitesEnabledDir,
		ReloadCommand:   []string{"nginx", "-s", "reload"},
	}
}

// GenerateConfig renders the nginx server block template and returns the config content.
// No filesystem writes occur; this is safe to call for preview / testing.
func GenerateConfig(vars ServerBlockVars) ([]byte, error) {
	if err := validateVars(vars); err != nil {
		return nil, err
	}
	if vars.GeneratedAt == "" {
		vars.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}

	tpl, err := template.New("server_block").Parse(serverBlockTpl)
	if err != nil {
		return nil, fmt.Errorf("parse nginx template: %w", err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, vars); err != nil {
		return nil, fmt.Errorf("execute nginx template: %w", err)
	}
	return buf.Bytes(), nil
}

// WriteAndReload writes the nginx config for hostname to SitesEnabledDir,
// then signals nginx to reload (nginx -s reload).
//
// Config file is written at: <SitesEnabledDir>/<sanitized-hostname>.conf
//
// The hostname is sanitized to a safe filename before use; any character outside
// [a-zA-Z0-9._-] is replaced with "_".
func (g *Generator) WriteAndReload(ctx context.Context, vars ServerBlockVars) (configPath string, err error) {
	if g.SitesEnabledDir == "" {
		g.SitesEnabledDir = DefaultSitesEnabledDir
	}

	content, err := GenerateConfig(vars)
	if err != nil {
		return "", err
	}

	filename := sanitizeFilename(vars.Hostname) + ".conf"
	configPath = filepath.Join(g.SitesEnabledDir, filename)

	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		return "", fmt.Errorf("write nginx config to %q: %w", configPath, err)
	}

	// Verify the config before reloading — prevents a bad config from taking nginx down.
	if err := g.testConfig(ctx); err != nil {
		// Roll back the written file on test failure.
		_ = os.Remove(configPath)
		return "", fmt.Errorf("nginx -t failed after writing config for %q: %w", vars.Hostname, err)
	}

	if err := g.reload(ctx); err != nil {
		return configPath, fmt.Errorf("nginx reload failed (config written to %q): %w", configPath, err)
	}

	return configPath, nil
}

// Remove removes the nginx config for hostname and reloads nginx.
func (g *Generator) Remove(ctx context.Context, hostname string) error {
	filename := sanitizeFilename(hostname) + ".conf"
	configPath := filepath.Join(g.SitesEnabledDir, filename)

	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove nginx config %q: %w", configPath, err)
	}

	if err := g.testConfig(ctx); err != nil {
		return fmt.Errorf("nginx -t failed after removing config for %q: %w", hostname, err)
	}
	return g.reload(ctx)
}

func (g *Generator) testConfig(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "nginx", "-t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (g *Generator) reload(ctx context.Context) error {
	rc := g.ReloadCommand
	if len(rc) == 0 {
		rc = []string{"nginx", "-s", "reload"}
	}
	cmd := exec.CommandContext(ctx, rc[0], rc[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx reload: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// validateVars checks that required template variables are present and do not
// contain characters that could enable config injection (newlines, null bytes, etc.).
func validateVars(v ServerBlockVars) error {
	if v.Hostname == "" {
		return fmt.Errorf("Hostname is required")
	}
	if v.TenantID == "" {
		return fmt.Errorf("TenantID is required")
	}
	if !uuidRe.MatchString(v.TenantID) {
		return fmt.Errorf("TenantID must be a valid UUID, got %q", v.TenantID)
	}
	if v.InstanceID == "" {
		return fmt.Errorf("InstanceID is required")
	}
	if !uuidRe.MatchString(v.InstanceID) {
		return fmt.Errorf("InstanceID must be a valid UUID, got %q", v.InstanceID)
	}
	if v.CertPath == "" {
		return fmt.Errorf("CertPath is required")
	}
	if v.KeyPath == "" {
		return fmt.Errorf("KeyPath is required")
	}
	if v.UpstreamHost == "" {
		return fmt.Errorf("UpstreamHost is required")
	}
	// Reject any field containing newlines or null bytes — config injection guard.
	for name, val := range map[string]string{
		"Hostname":     v.Hostname,
		"TenantID":     v.TenantID,
		"InstanceID":   v.InstanceID,
		"CertPath":     v.CertPath,
		"KeyPath":      v.KeyPath,
		"UpstreamHost": v.UpstreamHost,
	} {
		if strings.ContainsAny(val, "\n\r\x00") {
			return fmt.Errorf("field %s contains invalid characters (newline or null byte)", name)
		}
	}
	return nil
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// sanitizeFilename replaces any character outside [a-zA-Z0-9._-] with "_".
func sanitizeFilename(hostname string) string {
	safe := regexp.MustCompile(`[^a-zA-Z0-9._\-]`)
	return safe.ReplaceAllString(hostname, "_")
}
