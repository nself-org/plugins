package attest_test

// Purpose: replay every document under testdata/adversarial: each file holds
// an evidence document, the gate it is presented at and the reasons Verify and
// GateAccept must give. A new attack is a new file, not new code.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type advCase struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Gate        attest.Gate     `json:"gate"`
	Evidence    json.RawMessage `json:"evidence"`
	WantGate    []attest.Reason `json:"want_gate"`
	WantVerify  []attest.Reason `json:"want_verify"`
}

func TestGateAccept_AdversarialFixtures(t *testing.T) {
	attest.RegisterRemoteVerifier(nil)
	files, err := filepath.Glob("testdata/adversarial/*.json")
	if err != nil || len(files) < 10 {
		t.Fatalf("fixtures: %d %v", len(files), err)
	}
	for _, name := range files {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var c advCase
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		t.Run(c.Name, func(t *testing.T) {
			var ev model.Evidence
			if err := json.Unmarshal(c.Evidence, &ev); err != nil {
				t.Fatal(err)
			}
			if got := attest.GateAccept(ev, c.Gate); !sameReasons(got, c.WantGate) {
				t.Errorf("GateAccept: got %v want %v (%s)", got, c.WantGate, c.Description)
			}
			var verify []attest.Reason
			if _, err := attest.Verify(c.Evidence); err != nil {
				verify = rejection(t, err).Reasons
			} else {
				verify = []attest.Reason{}
			}
			if !sameReasons(verify, c.WantVerify) {
				t.Errorf("Verify: got %v want %v", verify, c.WantVerify)
			}
			if c.Name != "good" && len(c.WantGate) == 0 {
				t.Errorf("adversarial fixture %s is accepted", c.Name)
			}
		})
	}
}
