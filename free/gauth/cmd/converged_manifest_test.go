package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestGauthConvergedManifest checks the converged gauth manifest (P8-CONV-09).
// gauth is the CLI for the plugin-gauth server, so its plugin.json must name
// plugin-gauth in the description and in the GAUTH_URL and GAUTH_PORT
// descriptions. Server fields stay in free/plugin-gauth/plugin.json: no port
// (a copied port collides in scripts/conformance/ports.sh, see P8-CONV-26) and
// no NSELF_DB_URL, GAUTH_ENCRYPTION_KEY or GOOGLE_* env var.
func TestGauthConvergedManifest(t *testing.T) {
	raw, err := os.ReadFile("../plugin.json")
	if err != nil {
		t.Fatalf("read ../plugin.json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parse ../plugin.json: %v", err)
	}

	if v, _ := m["manifest_version"].(float64); v != 2 {
		t.Errorf("manifest_version is %v, want 2", m["manifest_version"])
	}
	if n, _ := m["name"].(string); n != "gauth" {
		t.Errorf("name is %q, want gauth", n)
	}
	if d, _ := m["description"].(string); !strings.Contains(d, "CLI for the plugin-gauth server") {
		t.Errorf("description %q must state that gauth is the CLI for the plugin-gauth server", d)
	}

	vars := manifestEnvVars(t, m)
	for _, name := range []string{"GAUTH_URL", "GAUTH_PORT"} {
		desc, ok := vars[name]
		switch {
		case !ok:
			t.Errorf("%s is missing from envVars", name)
		case strings.TrimSpace(desc) == "":
			t.Errorf("%s description is empty (must name plugin-gauth)", name)
		case !strings.Contains(desc, "plugin-gauth"):
			t.Errorf("%s description %q must name plugin-gauth", name, desc)
		}
	}
	for name := range vars {
		if name == "NSELF_DB_URL" || name == "GAUTH_ENCRYPTION_KEY" || strings.HasPrefix(name, "GOOGLE_") {
			t.Errorf("server env var %s must stay in free/plugin-gauth/plugin.json", name)
		}
	}

	if _, ok := m["port"]; ok {
		t.Errorf("top-level port must stay in free/plugin-gauth/plugin.json")
	}
	for _, key := range []string{"config", "service"} {
		if obj, ok := m[key].(map[string]any); ok {
			if _, has := obj["port"]; has {
				t.Errorf("%s.port must stay in free/plugin-gauth/plugin.json", key)
			}
		}
	}
}

// manifestEnvVars returns name -> description for every env var the manifest
// declares: the envVars array plus any env/envVars object maps (required,
// optional), which is where ports.sh looks for *_PORT defaults.
func manifestEnvVars(t *testing.T, m map[string]any) map[string]string {
	t.Helper()
	out := map[string]string{}
	add := func(v any) {
		switch x := v.(type) {
		case []any:
			for _, e := range x {
				switch ev := e.(type) {
				case map[string]any:
					n, _ := ev["name"].(string)
					d, _ := ev["description"].(string)
					if n != "" {
						out[n] = d
					}
				case string:
					out[ev] = ""
				}
			}
		case map[string]any:
			for _, section := range []string{"required", "optional"} {
				switch s := x[section].(type) {
				case map[string]any:
					for n := range s {
						if _, seen := out[n]; !seen {
							out[n] = ""
						}
					}
				case []any:
					for _, e := range s {
						if n, ok := e.(string); ok {
							if _, seen := out[n]; !seen {
								out[n] = ""
							}
						}
					}
				}
			}
		}
	}
	add(m["envVars"])
	add(m["env"])
	return out
}
