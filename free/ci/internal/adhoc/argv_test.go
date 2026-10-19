package adhoc

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

func modelJob() model.Job { return model.Job{} }

func TestRecordedArgvIsRedacted(t *testing.T) {
	r := redact.New()
	shaped := "ghp" + "_" + strings.Repeat("a1B2", 9) // assembled: no token literal in the source
	argv := []string{"deploy", "--token=PLANTEDVALUE123456", "--api-key", "ANOTHERPLANTED987654", "PASSWORD=hunter2hunter2", shaped, "--verbose", "go", "test", "./..."}
	Register(r, argv)
	got := Recorded(r, argv)
	for _, secret := range []string{"PLANTEDVALUE123456", "ANOTHERPLANTED987654", "hunter2hunter2", shaped} {
		if strings.Contains(got, secret) {
			t.Errorf("recorded argv holds %q: %s", secret, got)
		}
	}
	for _, word := range []string{"deploy", "--verbose", "go test ./..."} {
		if !strings.Contains(got, word) {
			t.Errorf("a plain word was hidden: %q missing from %s", word, got)
		}
	}
	// the registered values are masked in any output, in any form
	for _, secret := range []string{"PLANTEDVALUE123456", "ANOTHERPLANTED987654"} {
		f := r.NewFilter()
		out := string(append(f.Redact([]byte("line "+secret+" end\n")), f.Flush()...))
		if strings.Contains(out, secret) {
			t.Errorf("job output still holds %q: %s", secret, out)
		}
	}
}

func TestRecordedArgvIsBounded(t *testing.T) {
	long := strings.Repeat("x", 1000)
	if got := Recorded(redact.New(), []string{"echo", long}); len(got) > maxRecorded+len("…") {
		t.Fatalf("recorded %d bytes", len(got))
	}
}

// TestPositionalSecretsAreRedacted covers the forms where the secret has no
// secret-looking flag name in front of it (D-1020).
func TestPositionalSecretsAreRedacted(t *testing.T) {
	cases := []struct {
		name   string
		argv   []string
		secret string
		keep   []string // words that must stay visible
	}{
		{"mysql -p<value>", []string{"mysql", "-uroot", "-pS3cretPassw0rd", "-h", "db"}, "S3cretPassw0rd", []string{"mysql", "-uroot", "-h db"}},
		{"mysqldump behind sudo", []string{"sudo", "mysqldump", "-pS3cretPassw0rd", "app"}, "S3cretPassw0rd", []string{"mysqldump", "app"}},
		{"curl -u user:pass", []string{"curl", "-u", "deploy:Zx9PlantedPass", "https://example.org"}, "Zx9PlantedPass", []string{"curl", "deploy:", "https://example.org"}},
		{"curl --user=user:pass", []string{"curl", "--user=deploy:Zx9PlantedPass", "https://example.org"}, "Zx9PlantedPass", []string{"--user=deploy:"}},
		{"curl -uuser:pass", []string{"curl", "-udeploy:Zx9PlantedPass", "https://example.org"}, "Zx9PlantedPass", []string{"-udeploy:"}},
		{"header X-Api-Key", []string{"curl", "-H", "X-Api-Key: KeyPlanted123456", "https://example.org"}, "KeyPlanted123456", []string{"X-Api-Key:", "https://example.org"}},
		{"header Authorization", []string{"curl", "--header=Authorization: Basic QWxhZGRpbjpvcGVu", "https://example.org"}, "QWxhZGRpbjpvcGVu", []string{"Authorization:"}},
		{"header Cookie", []string{"curl", "-H", "Cookie: sid=SessionPlanted99", "https://example.org"}, "SessionPlanted99", []string{"Cookie:"}},
		{"json body", []string{"curl", "-d", `{"user":"bob","password":"JsonPlanted4567","x":1}`, "https://example.org"}, "JsonPlanted4567", []string{`"user":"bob"`, `"x":1`}},
		{"form body", []string{"curl", "-d", "user=bob&password=FormPlanted4567&x=1", "https://example.org"}, "FormPlanted4567", []string{"user=bob", "x=1"}},
		{"url userinfo", []string{"git", "clone", "https://bob:UrlPlanted4567@example.org/r.git"}, "UrlPlanted4567", []string{"https://bob:", "@example.org/r.git"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := redact.New()
			Register(r, c.argv)
			got := Recorded(r, c.argv)
			if strings.Contains(got, c.secret) {
				t.Errorf("recorded argv holds %q: %s", c.secret, got)
			}
			for _, w := range c.keep {
				if !strings.Contains(got, w) {
					t.Errorf("%q was hidden: %s", w, got)
				}
			}
			f := r.NewFilter()
			out := string(append(f.Redact([]byte("line "+c.secret+" end\n")), f.Flush()...))
			if strings.Contains(out, c.secret) {
				t.Errorf("job output still holds %q: %s", c.secret, out)
			}
		})
	}
}

// TestPlainArgvIsNotMasked guards the positive side: words that merely look
// like the positional forms stay visible.
func TestPlainArgvIsNotMasked(t *testing.T) {
	r := redact.New()
	argv := []string{"docker", "run", "-u", "1000:1000", "-p", "8080:80", "gcc", "-pthread", "-H", "Accept: application/json", "-d", `{"name":"x"}`, "--user", "bob"}
	Register(r, argv)
	got := Recorded(r, argv)
	if strings.Contains(got, "[redacted]") {
		t.Errorf("a plain command was masked: %s", got)
	}
}
