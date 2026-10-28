package protocol

import (
	"bytes"
	"strings"
	"testing"
)

func decodeOne(t *testing.T, raw string) (Message, error) {
	t.Helper()
	return NewDecoder(strings.NewReader(raw+"\n"), CarrierPolicy{Carrier: "B"}).Decode()
}

// An upgrade without a downgrade is the frame an agent without allow_downgrade already decodes; the field travels only when true.
func TestUpgradeAllowDowngradeIsAdditive(t *testing.T) {
	var plain, down bytes.Buffer
	if err := NewEncoder(&plain).Encode(Message{V: 1, Type: "upgrade", Seq: 1, Body: Upgrade{Version: "1.5.0", ManifestURL: "https://x/m.json"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain.String(), "allow_downgrade") {
		t.Fatalf("a plain upgrade names allow_downgrade: %s", plain.String())
	}
	if err := NewEncoder(&down).Encode(Message{V: 1, Type: "upgrade", Seq: 1, Body: Upgrade{Version: "1.4.0", ManifestURL: "https://x/m.json", AllowDowngrade: true}}); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{plain.String(), down.String()} {
		m, err := decodeOne(t, strings.TrimSpace(raw))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if got := m.Body.(*Upgrade).AllowDowngrade; got != strings.Contains(raw, "allow_downgrade") {
			t.Fatalf("%s: allow_downgrade = %v", raw, got)
		}
	}
}

func TestUpgradeResultFrame(t *testing.T) {
	good := map[string]UpgradeResult{
		`{"v":1,"type":"upgrade-result","seq":1,"body":{"version":"1.5.0","status":"done"}}`:                           {Version: "1.5.0", Status: UpgradeDone},
		`{"v":1,"type":"upgrade-result","seq":1,"body":{"version":"1.5.0","status":"refused","reason":"running_job"}}`: {Version: "1.5.0", Status: UpgradeRefused, Reason: "running_job"},
		`{"v":1,"type":"upgrade-result","seq":1,"body":{"version":"1.4.0","status":"refused","reason":"downgrade"}}`:   {Version: "1.4.0", Status: UpgradeRefused, Reason: "downgrade"},
		`{"v":1,"type":"upgrade-result","seq":1,"body":{"version":"2.0.0","status":"refused","reason":"ceiling"}}`:     {Version: "2.0.0", Status: UpgradeRefused, Reason: "ceiling"},
	}
	for raw, want := range good {
		m, err := decodeOne(t, raw)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if got := *m.Body.(*UpgradeResult); got != want {
			t.Fatalf("%s: got %+v", raw, got)
		}
	}
	for _, raw := range []string{
		`{"v":1,"type":"upgrade-result","seq":1,"body":{"version":"1.5.0"}}`,                                                    // no status
		`{"v":1,"type":"upgrade-result","seq":1,"body":{"status":"done"}}`,                                                      // no version
		`{"v":1,"type":"upgrade-result","seq":1,"body":{"version":"1.5.0","status":"maybe"}}`,                                   // unknown status
		`{"v":1,"type":"upgrade-result","seq":1,"body":{"version":"1.5.0","status":"refused"}}`,                                 // refused without a reason
		`{"v":1,"type":"upgrade-result","seq":1,"body":{"version":"1.5.0","status":"done","reason":"ceiling"}}`,                 // done with a reason
		`{"v":1,"type":"upgrade-result","seq":1,"body":{"version":"1.5.0","status":"refused","reason":"E666: token glrt-abc"}}`, // free text is not a key
		`{"v":1,"type":"upgrade-result","seq":1,"body":{"version":"1.5.0","status":"done","extra":1}}`,                          // strict decoding
	} {
		if _, err := decodeOne(t, raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
