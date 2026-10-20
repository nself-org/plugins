package usage

// Internal tests of AuthFrom and ghHost: the token source is built from the policy in hand (the CLI
// path installs no process-wide policy source), so token_env and the API host always match the URL
// the request goes to.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
)

func TestAuthFromUsesTheSettingsInHand(t *testing.T) {
	t.Setenv("NSELF_TEST_USAGE_TOKEN", "synthetic-test-token-0002")
	tok, err := AuthFrom(runtime.Settings{GitHubTokenEnv: "NSELF_TEST_USAGE_TOKEN", GitHubAPIURL: "https://ghe.example/api/v3"})(context.Background())
	if err != nil || tok != "synthetic-test-token-0002" {
		t.Fatalf("token_env must win: %q %v", tok, err)
	}
	t.Setenv("NSELF_TEST_USAGE_EMPTY", "")
	if _, err = AuthFrom(runtime.Settings{GitHubTokenEnv: "NSELF_TEST_USAGE_EMPTY"})(context.Background()); err == nil {
		t.Fatal("an unset token_env variable is E700, never a fallback to another credential")
	}
}

func TestGHHost(t *testing.T) {
	for in, want := range map[string]string{
		"https://api.github.com":     "github.com",
		"https://API.GitHub.com/":    "github.com",
		"https://ghe.example/api/v3": "ghe.example",
		"":                           "",
		"::not a url":                "",
	} {
		if got := ghHost(in); got != want {
			t.Errorf("ghHost(%q) = %q, want %q", in, got, want)
		}
	}
}
