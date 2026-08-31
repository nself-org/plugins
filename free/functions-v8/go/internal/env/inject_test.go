package env_test

import (
	"testing"

	"github.com/nself-org/nself-functions-v8/internal/env"
)

func TestEnvVarInjection_AllowlistOnly(t *testing.T) {
	// Simulate host env with a secret and an allowed key.
	allEnv := map[string]string{
		"SECRET_TOKEN": "supersecret-host-value",
		"API_KEY":      "xyz-allowed",
		"DATABASE_URL": "postgres://internal",
	}
	allowed := []string{"API_KEY"}

	result := env.BuildEnv(allEnv, allowed)

	if _, ok := result["SECRET_TOKEN"]; ok {
		t.Error("SECRET_TOKEN must NOT be in the injected env (not in allowlist)")
	}
	if _, ok := result["DATABASE_URL"]; ok {
		t.Error("DATABASE_URL must NOT be in the injected env (not in allowlist)")
	}
	if v, ok := result["API_KEY"]; !ok || v != "xyz-allowed" {
		t.Errorf("API_KEY should be injected, got: %q (ok=%v)", v, ok)
	}
}

func TestEnvVarInjection_EmptyAllowlist(t *testing.T) {
	allEnv := map[string]string{
		"SECRET": "value",
	}
	result := env.BuildEnv(allEnv, []string{})
	if len(result) != 0 {
		t.Errorf("expected empty env with empty allowlist, got: %v", result)
	}
}

func TestLogRedaction_SecretPatterns(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
		absent   string
	}{
		{
			name:     "MY_KEY redacted",
			input:    "connected with MY_KEY=supersecret",
			contains: "[REDACTED]",
			absent:   "supersecret",
		},
		{
			name:     "API_SECRET redacted",
			input:    "API_SECRET=abc123 used",
			contains: "[REDACTED]",
			absent:   "abc123",
		},
		{
			name:     "DB_PASSWORD redacted",
			input:    "DB_PASSWORD=mypass123",
			contains: "[REDACTED]",
			absent:   "mypass123",
		},
		{
			name:     "non-secret unchanged",
			input:    "hello world",
			contains: "hello world",
			absent:   "[REDACTED]",
		},
		{
			name:     "TOKEN redacted",
			input:    "AUTH_TOKEN=tok-xyz",
			contains: "[REDACTED]",
			absent:   "tok-xyz",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := env.RedactSecrets(tc.input)
			if tc.contains != "" {
				found := false
				if len(result) >= len(tc.contains) {
					for i := 0; i <= len(result)-len(tc.contains); i++ {
						if result[i:i+len(tc.contains)] == tc.contains {
							found = true
							break
						}
					}
				}
				if !found {
					t.Errorf("expected %q in result, got: %q", tc.contains, result)
				}
			}
			if tc.absent != "" {
				for i := 0; i <= len(result)-len(tc.absent) && i >= 0; i++ {
					if i+len(tc.absent) <= len(result) && result[i:i+len(tc.absent)] == tc.absent {
						t.Errorf("expected %q to be absent from result, got: %q", tc.absent, result)
						break
					}
				}
			}
		})
	}
}
