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

func TestWellKnownSecretForms(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
	}{
		{"mysql", []string{"mysql", "-p" + "fixture-value"}},
		{"curl-user", []string{"curl", "-u", "user:fixture-value"}},
		{"header", []string{"curl", "-H", "X-Api-Key: fixture-value"}},
		{"json", []string{"curl", "-d", `{"password":"fixture-value"}`}},
		{"nested-json", []string{"curl", "-d", `{"data":[{"api_key":"fixture-value"}]}`}},
		{"long-user", []string{"curl", "--user=user:fixture-value"}},
		{"short-header", []string{"curl", "-H", "X-Api-Key: k"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := redact.New()
			Register(r, tc.argv)
			got := Recorded(r, tc.argv)
			if strings.Contains(got, "fixture-value") || strings.Contains(got, "X-Api-Key: k") {
				t.Fatal("recorded command contains a sensitive value")
			}
			if tc.name != "short-header" {
				f := r.NewFilter()
				masked := string(append(f.Redact([]byte("value: fixture-value\n")), f.Flush()...))
				if strings.Contains(masked, "fixture-value") {
					t.Fatal("job output contains an extracted sensitive value")
				}
			}
			if !strings.HasPrefix(got, tc.argv[0]+" ") {
				t.Fatal("command name was lost")
			}
		})
	}
}

func TestWellKnownFormsKeepPlainArguments(t *testing.T) {
	for _, argv := range [][]string{
		{"echo", "-plain"},
		{"curl", "-H", "Content-Type: application/json"},
		{"curl", "-d", `{"name":"example"}`},
	} {
		r := redact.New()
		Register(r, argv)
		if got := Recorded(r, argv); got != strings.Join(argv, " ") {
			t.Fatal("plain arguments changed")
		}
	}
}
