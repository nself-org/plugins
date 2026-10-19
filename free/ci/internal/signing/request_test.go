package signing_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
	"github.com/nself-org/plugins/free/ci/internal/signing"
)

func decode(s string) error {
	_, _, err := signing.DecodeRequest(strings.NewReader(s))
	return err
}

func TestDecodeRequestRefusals(t *testing.T) {
	ok := `{"run_id":"r","items":[{"payload_type":"` + signing.PayloadStatement + `","payload_b64":"eA=="}]}`
	if err := decode(ok); err != nil {
		t.Fatal(err)
	}
	many := make([]signing.Item, signing.MaxItems+1)
	for i := range many {
		many[i] = signing.Item{PayloadType: manifest.PayloadType, PayloadB64: "eA=="}
	}
	manyJSON, _ := json.Marshal(signing.Request{RunID: "r", Items: many})
	atMax, _ := json.Marshal(signing.Request{RunID: "r", Items: many[:signing.MaxItems]})
	if err := decode(string(atMax)); err != nil {
		t.Fatalf("%d items refused: %v", signing.MaxItems, err)
	}
	big := `{"run_id":"r","items":[{"payload_type":"` + signing.PayloadStatement + `","payload_b64":"` + strings.Repeat("A", signing.MaxRequestBytes) + `"}]}`
	for name, in := range map[string]string{
		"empty":         ``,
		"not json":      `nope`,
		"unknown field": `{"run_id":"r","items":[],"extra":1}`,
		"item field":    `{"run_id":"r","items":[{"payload_type":"` + signing.PayloadStatement + `","payload_b64":"eA==","x":1}]}`,
		"two docs":      ok + ok,
		"no run":        `{"items":[{"payload_type":"` + signing.PayloadStatement + `","payload_b64":"eA=="}]}`,
		"no items":      `{"run_id":"r","items":[]}`,
		"too many":      string(manyJSON),
		"bad type":      `{"run_id":"r","items":[{"payload_type":"text/plain","payload_b64":"eA=="}]}`,
		"bad base64":    `{"run_id":"r","items":[{"payload_type":"` + signing.PayloadStatement + `","payload_b64":"!!"}]}`,
		"url base64":    `{"run_id":"r","items":[{"payload_type":"` + signing.PayloadStatement + `","payload_b64":"_-_-"}]}`,
		"empty payload": `{"run_id":"r","items":[{"payload_type":"` + signing.PayloadStatement + `","payload_b64":""}]}`,
		"over 4 MiB":    big,
	} {
		err := decode(in)
		if !signing.IsReason(err, signing.ReasonPayloadRejected) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestDecodeRequestSizeBoundary(t *testing.T) {
	head := `{"run_id":"r","items":[{"payload_type":"` + signing.PayloadStatement + `","payload_b64":"`
	tail := `"}]}`
	pad := signing.MaxRequestBytes - len(head) - len(tail)
	pad -= pad % 4
	exact := head + strings.Repeat("A", pad) + tail
	if err := decode(exact); err != nil {
		t.Fatalf("a request of %d bytes was refused: %v", len(exact), err)
	}
	// pad the document with whitespace to exactly the cap, then one more byte
	atCap := exact + strings.Repeat(" ", signing.MaxRequestBytes-len(exact))
	if err := decode(atCap); err != nil {
		t.Fatalf("a request of exactly the cap was refused: %v", err)
	}
	if err := decode(atCap + " "); !signing.IsReason(err, signing.ReasonPayloadRejected) {
		t.Fatalf("a request one byte over the cap was accepted: %v", err)
	}
}

func TestErrorShape(t *testing.T) {
	err := signing.Failf(signing.ReasonKeyMissing, "x %d", 1)
	if err.Error() != "E717 key_missing: x 1" {
		t.Fatal(err)
	}
	e := err.(*signing.Error)
	if e.Code() != "E717" || !e.Is(&signing.Error{}) || !e.Is(&signing.Error{Reason: signing.ReasonKeyMissing}) || e.Is(&signing.Error{Reason: signing.ReasonKeyInvalid}) {
		t.Fatal("Is")
	}
	if signing.IsReason(nil, signing.ReasonKeyMissing) || signing.IsReason(err, signing.ReasonKeyInvalid) {
		t.Fatal("IsReason")
	}
	long := decode(`{"run_id":"r","items":[{"payload_type":"` + strings.Repeat("a", 200) + "\\u0001" + `","payload_b64":"eA=="}]}`).Error()
	if !strings.Contains(long, strings.Repeat("a", 64)+`..."`) || strings.Contains(long, strings.Repeat("a", 65)) {
		t.Fatalf("an input echo was not bounded to 64 bytes: %q", long)
	}
	ctl := decode(`{"run_id":"r","items":[{"payload_type":"a\u0001\u00e9b","payload_b64":"eA=="}]}`).Error()
	if !strings.Contains(ctl, `"a?`) {
		t.Fatalf("control and non-ASCII bytes were echoed: %q", ctl)
	}
}
