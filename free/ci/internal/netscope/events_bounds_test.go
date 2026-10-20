package netscope

import (
	"bytes"
	"strings"
	"testing"
)

// A job can make the sidecar log one very long line (a long CONNECT host); the denials after it still count.
func TestParseEventsSurvivesAnOversizeLine(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(Event{Scope: "internet", Host: "first", Reason: ReasonSpecial}.Encode())
	buf.WriteString(`{"kind":"network.denied","host":"` + strings.Repeat("a", 3*maxEventLine) + "\"}\n")
	buf.Write(Event{Scope: "internet", Host: "last", Reason: ReasonSpecial}.Encode())
	got := ParseEvents(&buf)
	if len(got) != 2 || got[0].Host != "first" || got[1].Host != "last" {
		t.Fatalf("%+v", got)
	}
}

func TestProxyEventsAreBoundedAndCarryNoReasonToTheJob(t *testing.T) {
	var ev syncBuf
	p := &Proxy{Guard: Guard{Scope: "internet"}, Events: &ev}
	long := strings.Repeat("h", 5000)
	for i := 0; i < maxEvents+50; i++ {
		p.record(long, "443", Verdict{Reason: ReasonSpecial, Detail: long})
	}
	got := ParseEvents(bytes.NewReader(ev.bytes()))
	if len(got) != maxEvents+1 {
		t.Fatalf("%d events, want %d plus the drop marker", len(got), maxEvents)
	}
	if got[0].Host != strings.Repeat("h", maxEventHost) || got[len(got)-1].Reason != ReasonEventsDropped {
		t.Errorf("first host %d bytes, last %+v", len(got[0].Host), got[len(got)-1])
	}
}
