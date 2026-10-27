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

func TestDashPrefixedSecretValuesAreRedacted(t *testing.T) {
	for _, secret := range []string{"-randomValue123", "--randomValue123", "-x"} {
		t.Run(secret, func(t *testing.T) {
			argv := []string{"deploy", "--password", secret, "--verbose"}
			if got := Recorded(redact.New(), argv); got != "deploy --password [redacted] --verbose" {
				t.Errorf("recorded argv before registration = %q", got)
			}
			r := redact.New()
			tooShort := Register(r, argv)
			if len(secret) < redact.MinValueLen {
				if tooShort != 1 {
					t.Errorf("short secret count = %d, want 1", tooShort)
				}
				return
			}
			if tooShort != 0 {
				t.Fatalf("short secret count = %d, want 0", tooShort)
			}
			if got := Recorded(r, argv); got != "deploy --password [redacted] --verbose" {
				t.Errorf("recorded argv after registration = %q", got)
			}
			f := r.NewFilter()
			got := string(append(f.Redact([]byte("line "+secret+" end\n")), f.Flush()...))
			if strings.Contains(got, secret) || !strings.Contains(got, redact.Placeholder) {
				t.Errorf("job output was not redacted: %q", got)
			}
		})
	}
}

func TestWellKnownSecretFormsAreRedacted(t *testing.T) {
	cases := []struct {
		name   string
		argv   []string
		secret string
	}{
		{"mysql -pVALUE", []string{"mysql", "-pS3cretPass", "db"}, "S3cretPass"},
		{"mariadb path -pVALUE", []string{"/usr/bin/mariadb", "-pS3cretPass", "db"}, "S3cretPass"},
		{"mysql behind docker exec", []string{"docker", "exec", "db", "mysql", "-pS3cretPass"}, "S3cretPass"},
		{"curl -u user:pass", []string{"curl", "-u", "admin:s3cret", "https://x"}, "admin:s3cret"},
		{"header X-Api-Key", []string{"curl", "-H", "X-Api-Key: tok123456789", "https://x"}, "tok123456789"},
		{"header Authorization", []string{"curl", "-H", "Authorization: Bearer abcdefghij", "https://x"}, "Bearer abcdefghij"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := redact.New()
			Register(r, tc.argv)
			got := Recorded(r, tc.argv)
			if strings.Contains(got, tc.secret) {
				t.Errorf("recorded argv holds %q: %s", tc.secret, got)
			}
			f := r.NewFilter()
			out := string(append(f.Redact([]byte("line "+tc.secret+" end\n")), f.Flush()...))
			if strings.Contains(out, tc.secret) {
				t.Errorf("job output still holds %q: %s", tc.secret, out)
			}
		})
	}
}

func TestNonSecretDashFlagsNotRedacted(t *testing.T) {
	// -port, -pidfile, -print, -path and plain positional args must stay visible,
	// in the recorded line and in job output.
	argv := []string{"myapp", "-port=8080", "-pidfile=/tmp/pid", "-print", "-path=/srv/app", "plainarg"}
	r := redact.New()
	Register(r, argv)
	got := Recorded(r, argv)
	if want := strings.Join(argv, " "); got != want {
		t.Errorf("recorded argv = %q, want %q", got, want)
	}
	f := r.NewFilter()
	out := string(append(f.Redact([]byte("pidfile=/tmp/pid print path=/srv/app port=8080 plainarg\n")), f.Flush()...))
	if strings.Contains(out, redact.Placeholder) {
		t.Errorf("job output was masked: %q", out)
	}
}

// A -pVALUE too short for the registry still has to be hidden in Recorded.
func TestShortMySQLPasswordMaskedInRecorded(t *testing.T) {
	argv := []string{"mysql", "-pab", "db"}
	r := redact.New()
	if tooShort := Register(r, argv); tooShort != 1 {
		t.Fatalf("short secret count = %d, want 1", tooShort)
	}
	if got, want := Recorded(r, argv), "mysql -p[redacted] db"; got != want {
		t.Errorf("recorded argv = %q, want %q", got, want)
	}
}

// Flags that merely start with -p belong to other tools (go test, make...) and
// must stay visible: redacting them would corrupt the job's output.
func TestDashPFlagsOfOtherToolsAreNotRedacted(t *testing.T) {
	argv := []string{"go", "test", "-parallel", "4", "-pkgdir=/tmp/pkg", "-print-errors", "-p4", "./..."}
	r := redact.New()
	Register(r, argv)
	got := Recorded(r, argv)
	if want := strings.Join(argv, " "); got != want {
		t.Errorf("recorded argv = %q, want %q", got, want)
	}
	f := r.NewFilter()
	out := string(append(f.Redact([]byte("ok parallel pkgdir print-errors\n")), f.Flush()...))
	if strings.Contains(out, redact.Placeholder) {
		t.Errorf("job output was masked: %q", out)
	}
}
