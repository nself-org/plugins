package attest_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestPublicationDiagnosticFields(t *testing.T) {
	const marker = "10.1.2.3"
	for _, field := range []string{"diagnosis", "remediation", "flaky source", "retry diagnosis", "retry excerpt"} {
		t.Run(field, func(t *testing.T) {
			ev := leaky()
			ev.Checks = []model.Check{{}}
			ev.Retries = []model.Retry{{}}
			switch field {
			case "diagnosis":
				ev.Checks[0].Diagnosis = marker
			case "remediation":
				ev.Checks[0].Remediation = marker
			case "flaky source":
				ev.Checks[0].Flaky = &model.Flaky{Source: marker}
			case "retry diagnosis":
				ev.Retries[0].Diagnosis = marker
			case "retry excerpt":
				ev.Retries[0].Excerpt = marker
			}
			before, err := json.Marshal(ev)
			if err != nil {
				t.Fatal(err)
			}
			after, err := json.Marshal(attest.Publish(ev, attest.Public))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(after), marker) {
				t.Fatal("private diagnostic published")
			}
			unchanged, err := json.Marshal(ev)
			if err != nil || string(unchanged) != string(before) {
				t.Fatal("input modified")
			}
		})
	}
}

func TestPublicationExoticAddresses(t *testing.T) {
	for _, marker := range []string{"0x0a000001", "167772161", "012.0.0.1", "10.1", "::ffff:10.0.0.9", "/private/var/folders/zz"} {
		t.Run(marker, func(t *testing.T) {
			if got := (attest.Profile{}).Text("address=" + marker); strings.Contains(got, marker) {
				t.Fatal("private address published")
			}
		})
	}
}
