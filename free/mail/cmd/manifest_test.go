// Purpose: prove the mail manifest carries the fields merged from the free
// email plugin (P8-CONV-05) and that the merge stayed additive.
//
// Inputs: ../plugin.json (read only).
//
// Outputs: test results.
//
// Constraints: the Elastic Email settings are optional (required: false,
// written out, not left to the zero value) because mail still sends through
// ping_api and no Elastic Email adapter reads them yet. The original env vars
// and tags must survive the merge.
package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type mailManifestEnvVar struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    *bool  `json:"required"`
}

func TestMailManifestMergesEmailFields(t *testing.T) {
	raw, err := os.ReadFile("../plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Name    string               `json:"name"`
		EnvVars []mailManifestEnvVar `json:"envVars"`
		Tags    []string             `json:"tags"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("free/mail/plugin.json: %v", err)
	}
	if m.Name != "mail" {
		t.Fatalf("free/mail/plugin.json name = %q, want mail", m.Name)
	}

	env := map[string]mailManifestEnvVar{}
	for _, v := range m.EnvVars {
		env[v.Name] = v
	}
	merged := []string{"ELASTIC_EMAIL_API_KEY", "ELASTIC_EMAIL_FROM"}
	var missing []string
	for _, name := range merged {
		if _, ok := env[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("free/mail/plugin.json envVars is missing %s", strings.Join(missing, ", "))
	}
	for _, name := range merged {
		v := env[name]
		if v.Required == nil || *v.Required {
			t.Errorf("free/mail/plugin.json envVars %s: required must be written as false", name)
		}
		if strings.TrimSpace(v.Description) == "" {
			t.Errorf("free/mail/plugin.json envVars %s: description is empty", name)
		}
	}

	for _, name := range []string{"NSELF_PING_API_URL", "NSELF_PLUGIN_LICENSE_KEY", "NSELF_LICENSE_KEY_1"} {
		if _, ok := env[name]; !ok {
			t.Errorf("free/mail/plugin.json envVars lost the original %s", name)
		}
	}
	tags := map[string]bool{}
	for _, tag := range m.Tags {
		tags[tag] = true
	}
	for _, tag := range []string{"mail", "email", "postmark", "transactional", "broadcast", "elastic-email"} {
		if !tags[tag] {
			t.Errorf("free/mail/plugin.json tags is missing %q", tag)
		}
	}
}
