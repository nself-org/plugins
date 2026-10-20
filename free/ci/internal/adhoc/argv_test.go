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

func TestDashPrefixedSecretValues(t *testing.T) {
	for _, value := range []string{"-synthetic-value", "--synthetic-value", "-x"} {
		t.Run(value, func(t *testing.T) {
			r := redact.New()
			argv := []string{"deploy", "--token", value}
			Register(r, argv)
			if strings.Contains(Recorded(r, argv), value) {
				t.Fatal("recorded argv exposes secret value")
			}
			if len(value) >= redact.MinValueLen {
				f := r.NewFilter()
				if strings.Contains(string(append(f.Redact([]byte(value)), f.Flush()...)), value) {
					t.Fatal("output exposes secret value")
				}
			}
		})
	}
}
