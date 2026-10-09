package protocol

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestPoisonAfterOversizeOrMalformed(t *testing.T) {
	good := `{"v":1,"type":"ack","seq":1,"body":{"ack_seq":1}}` + "\n"
	for name, first := range map[string]string{"oversize": strings.Repeat("x", MaxFrame+1) + "\n", "malformed": "{\n"} {
		t.Run(name, func(t *testing.T) {
			d := NewDecoder(strings.NewReader(first+good), CarrierPolicy{})
			_, err := d.Decode()
			if err == nil {
				t.Fatal("accepted bad frame")
			}
			_, again := d.Decode()
			if again != err {
				t.Fatalf("decoder resynchronized: %v, %v", err, again)
			}
		})
	}
}

func TestRejectDuplicateAndCaseVariantKeys(t *testing.T) {
	for _, raw := range []string{
		`{"v":1,"v":1,"type":"ack","seq":1,"body":{"ack_seq":1}}`,
		`{"v":1,"type":"ack","TYPE":"ack","seq":1,"body":{"ack_seq":1}}`,
		`{"v":1,"type":"ack","seq":1,"body":{"ack_seq":1,"ACK_SEQ":1}}`,
		`{"v":1,"type":"ack","seq":1,"body":{"ack_seq":1,"ack_ſeq":1}}`,
		`{"v":1,"type":"secrets","seq":1,"body":{"lease_id":"l","values":[{"ref":"project/x","value":"a","VALUE":"b"}]}}`,
	} {
		if _, err := NewDecoder(strings.NewReader(raw+"\n"), CarrierPolicy{Carrier: "A", Personal: true, OperatorOwned: true}).Decode(); err == nil {
			t.Fatal(raw)
		}
	}
}

func TestRejectMissingAndNullBodies(t *testing.T) {
	for _, raw := range []string{
		`{"v":1,"type":"hello","seq":1,"body":null}`,
		`{"v":1,"type":"hello","seq":1,"body":{}}`,
		`{"v":1,"type":"ack","seq":1,"body":{}}`,
		`{"v":1,"type":"ack","seq":1}`,
	} {
		if _, err := NewDecoder(strings.NewReader(raw+"\n"), CarrierPolicy{}).Decode(); err == nil {
			t.Fatal(raw)
		}
	}
}

func TestLeaseScopedFencing(t *testing.T) {
	for _, kind := range []string{"log", "artifact-ref", "cache-ref", "cancel", "lease", "result"} {
		var body any
		switch kind {
		case "log":
			body = Log{LeaseID: "body", LineSeq: 1, Stream: "stdout", Text: "x"}
		case "artifact-ref":
			body = ArtifactRef{LeaseID: "body", Path: "p", SHA256: "sha", Size: 1}
		case "cache-ref":
			body = CacheRef{LeaseID: "body", Spec: "s", Key: "k", Blobs: []CacheBlob{}}
		case "cancel":
			body = Cancel{LeaseID: "body", Reason: "stop"}
		case "lease":
			body = Lease{LeaseID: "body", Epoch: 1, CompatMode: Compat15}
		case "result":
			body = Result{LeaseID: "body", Epoch: 1}
		}
		for _, mismatch := range []bool{false, true} {
			m := frame(kind, 1, body)
			m.LeaseID = "body"
			if mismatch {
				m.LeaseID = "other"
			}
			raw, _ := json.Marshal(m)
			if _, err := NewDecoder(bytes.NewReader(append(raw, '\n')), CarrierPolicy{}).Decode(); err == nil {
				t.Fatalf("accepted %s mismatch=%v", kind, mismatch)
			}
		}
	}
}
func TestNegotiateSupportedStrictVersion(t *testing.T) {
	for _, version := range []string{"1.4.0junk", "v1.4.0-pre", "1.4", "1.4.0+meta"} {
		if v, r := Negotiate([]int{1}, []int{1}, version, "1.4.0"); v != 0 || r == nil {
			t.Fatal(version, v, r)
		}
	}
	if v, r := Negotiate([]int{2}, []int{2}, "1.5.0", "1.4.0"); v != 0 || r == nil {
		t.Fatal(v, r)
	}
}

func TestResumeSequenceState(t *testing.T) {
	raw := `{"v":1,"type":"ack","seq":45,"body":{"ack_seq":44}}` + "\n"
	d, err := NewResumedDecoder(strings.NewReader(raw), CarrierPolicy{}, 44, 44)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.Decode(); err != nil {
		t.Fatal(err)
	}
	if _, err = NewResumedDecoder(strings.NewReader(raw), CarrierPolicy{}, 44, 43); err == nil {
		t.Fatal("accepted inconsistent resume")
	}
	var wire bytes.Buffer
	e, err := NewResumedEncoder(&wire, 44, 44)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Encode(frame("ack", 45, Ack{AckSeq: 44})); err != nil {
		t.Fatal(err)
	}
	if _, err := NewResumedEncoder(&wire, 44, 43); err == nil {
		t.Fatal("accepted inconsistent outgoing resume")
	}
}

func TestHandshakePinBeforeWelcome(t *testing.T) {
	a, b := "old", "new"
	answer, _ := AcceptHello(Hello{Versions: []int{1}, AgentVersion: "1.5.0", AgeRecipient: &b}, &a, []int{1}, "1.4.0", "s", 1000, 3000)
	if _, ok := answer.(Welcome); ok {
		t.Fatal("welcome after pin mismatch")
	}
	if r, ok := answer.(Reject); !ok || r.Code != "E660" {
		t.Fatal(answer)
	}
	var wire bytes.Buffer
	if err := WriteHelloResponse(NewEncoder(&wire), Hello{Versions: []int{1}, AgentVersion: "1.5.0", AgeRecipient: &b}, &a, []int{1}, "1.4.0", "s", 1000, 3000); err != nil {
		t.Fatal(err)
	}
	m, err := NewDecoder(&wire, CarrierPolicy{}).Decode()
	if err != nil || m.Type != "reject" {
		t.Fatal(m, err)
	}
}

func TestProtocolSchemaCodecAgreement(t *testing.T) {
	schema, err := Schema()
	if err != nil {
		t.Fatal(err)
	}
	for name := range goldenCases() {
		data, err := os.ReadFile(filepath.Join("testdata", "transcripts", name+".ndjson"))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
			if err := model.ValidateJSON(schema, line); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
	for _, raw := range []string{
		`{"v":1,"type":"lease","seq":1,"body":{}}`,
		`{"v":1,"type":"chunk","seq":1,"body":{"stream":"cache","offset":0,"data":"x","final":true}}`,
		`{"v":1,"type":"ack","seq":1,"body":{}}`,
	} {
		if err := model.ValidateJSON(schema, []byte(raw)); err == nil {
			t.Fatal("schema accepted", raw)
		}
		if _, err := NewDecoder(strings.NewReader(raw+"\n"), CarrierPolicy{}).Decode(); err == nil {
			t.Fatal("codec accepted", raw)
		}
	}
}
