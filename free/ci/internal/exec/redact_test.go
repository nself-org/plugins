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

// byteByByte feeds the input one byte per Redact call.
func byteByByte(in string) string {
	r := NewOutputRedactor()
	var out []byte
	for i := 0; i < len(in); i++ {
		out = append(out, r.Redact([]byte{in[i]})...)
	}
	return string(append(out, r.Flush()...))
}

// A named secret longer than maxHold must stay masked across the bounded line
// cut, whatever the chunking (D-0982).
func TestOutputRedactorKeepsMaskAcrossLineCut(t *testing.T) {
	long := strings.Repeat("abcdefghij", 500)
	words := strings.TrimSpace(strings.Repeat("alpha beta ", 450))
	cases := map[string]string{
		"unquoted":      "TOKEN=" + long + "\nnext\n",
		"unquoted tail": "pre " + strings.Repeat("w ", 300) + "SECRET_KEY=" + long + " after\n",
		"double quoted": `API_KEY="` + words + `" after` + "\n",
		"single quoted": "PASSWORD='" + words + "' after\n",
		"empty at cut":  strings.Repeat("a", 4096-len("TOKEN=")) + "TOKEN=" + long + "\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			whole := redactAll(in)
			if strings.Contains(whole, "abcdefghij") || strings.Contains(whole, "alpha") {
				t.Fatalf("whole write leaked: %.80q", whole)
			}
			for chunk, got := range map[string]string{"byte": byteByByte(in), "split": redactAll(in[:len(in)*3/5], in[len(in)*3/5:])} {
				if strings.Contains(got, "abcdefghij") || strings.Contains(got, "alpha") || strings.Contains(got, "beta") {
					t.Errorf("%s writes leaked the value suffix: %.120q", chunk, got)
				}
				if got != whole {
					t.Errorf("%s writes differ from the whole write: %.80q vs %.80q", chunk, got, whole)
				}
			}
		})
	}
}
