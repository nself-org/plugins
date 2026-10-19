package events

import "testing"

func TestRecordRefRoundTrips(t *testing.T) {
	in := Record{Type: "check_result", Job: "build", Attempt: "a1", Data: map[string]any{"state": "passed", "exit_code": float64(0)}}
	out, err := ParseRecord(string(in.Type), in.Ref())
	if err != nil || out.Type != in.Type || out.Job != "build" || out.Attempt != "a1" || out.Data["state"] != "passed" || out.Data["exit_code"] != float64(0) {
		t.Fatalf("%+v %v", out, err)
	}
}

func TestParseRecordRefusesWhatIsNotARecord(t *testing.T) {
	if _, err := ParseRecord("queued", "not json"); err == nil {
		t.Fatal("garbage ref accepted")
	}
	if _, err := ParseRecord("nonsense", "{}"); err == nil {
		t.Fatal("unknown event type accepted")
	}
	if r, err := ParseRecord("queued", "{}"); err != nil || r.Data == nil {
		t.Fatalf("an empty ref is a record with empty data: %+v %v", r, err)
	}
}
