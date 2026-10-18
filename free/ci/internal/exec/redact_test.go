package exec

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func redactAll(chunks ...string) string {
	r := NewOutputRedactor()
	var out []byte
	for _, c := range chunks {
		out = append(out, r.Redact([]byte(c))...)
	}
	return string(append(out, r.Flush()...))
}

func TestOutputRedactorMasksSecrets(t *testing.T) {
	ghp := "ghp_" + strings.Repeat("Zz9", 10)
	cases := map[string]string{
		"API_TOKEN=abc123def\n":                    "API_TOKEN=[redacted]\n",
		`export DB_PASSWORD="two words"` + "\n":    "export DB_PASSWORD=[redacted]\n",
		"password: hunter2\n":                      "password: [redacted]\n",
		"Authorization: Bearer abcdefgh12345678\n": "Authorization: Bearer [redacted]\n",
		"clone https://bot:s3cret@example.com/r\n": "clone https://bot:[redacted]@example.com/r\n",
		"key AKIAABCDEFGHIJKLMNOP end\n":           "key [redacted] end\n",
		"tok " + ghp + "\n":                        "tok [redacted]\n",
		"plain build output 100%\n":                "plain build output 100%\n",
		"-----BEGIN RSA PRIVATE KEY-----\nMIIabc\n-----END RSA PRIVATE KEY-----\nok\n": "[redacted private key]\nok\n",
	}
	for in, want := range cases {
		if got := redactAll(in); got != want {
			t.Errorf("redact(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOutputRedactorSecretSplitAcrossChunks(t *testing.T) {
	got := redactAll("GITHUB_TO", "KEN=abcdef", "ghijkl\nnext")
	if strings.Contains(got, "abcdef") || !strings.Contains(got, "GITHUB_TOKEN=[redacted]\nnext") {
		t.Fatalf("got %q", got)
	}
}

func TestOutputRedactorBoundsALongLine(t *testing.T) {
	r := NewOutputRedactor()
	got := string(r.Redact([]byte(strings.Repeat("word ", 2000) + "SECRET_KEY=topsecretvalue")))
	if len(got) == 0 || len(r.(*lineRedactor).held) > maxHold {
		t.Fatalf("emitted %d held %d", len(got), len(r.(*lineRedactor).held))
	}
	if all := got + string(r.Flush()); strings.Contains(all, "topsecretvalue") {
		t.Fatal("secret leaked after the cut")
	}
}

func TestAllowlistEnvDropsOperatorSecrets(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("LC_ALL", "C")
	t.Setenv("MY_API_TOKEN", "x")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "y")
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent")
	env, err := AllowlistEnv{}.Environment(context.Background(), model.Job{})
	if err != nil {
		t.Fatal(err)
	}
	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, want := range []string{"\nPATH=/usr/bin\n", "\nLC_ALL=C\n"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, bad := range []string{"MY_API_TOKEN", "AWS_SECRET_ACCESS_KEY", "SSH_AUTH_SOCK"} {
		if strings.Contains(joined, bad) {
			t.Errorf("leaked %s", bad)
		}
	}
}
