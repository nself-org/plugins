package detach

import (
	"bytes"
	"strings"
	"testing"
)

func seqs(t *testing.T, out string) (types []string, seq []int64) {
	t.Helper()
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		r, ok := ParseLine([]byte(l))
		if !ok {
			t.Fatalf("not a record: %q", l)
		}
		types, seq = append(types, r.Type), append(seq, r.Seq)
	}
	return
}

func TestSynthStream(t *testing.T) {
	var out bytes.Buffer
	s := NewSynth(&out, "r1", 0)
	s.Observe(View{RunID: "r1", State: "running", Jobs: []JobState{{Name: "build", State: "queued"}, {Name: "test", State: "queued"}}})
	s.Observe(View{RunID: "r1", State: "running", Jobs: []JobState{{Name: "build", State: "running"}, {Name: "test", State: "queued"}}})
	s.Observe(View{RunID: "r1", State: "running", Jobs: []JobState{{Name: "build", State: "running"}, {Name: "test", State: "queued"}}}) // no change, no record
	s.Finish(View{RunID: "r1", State: "failed", Jobs: []JobState{{Name: "build", State: "passed"}, {Name: "test", State: "failed", FailureClass: "code"}}},
		map[string]any{"result": "fail"})
	types, seq := seqs(t, out.String())
	want := []string{"queued", "started", "check_result", "check_result", "finished", "result"}
	if strings.Join(types, ",") != strings.Join(want, ",") {
		t.Fatalf("types %v, want %v", types, want)
	}
	for i := 1; i < len(seq); i++ {
		if seq[i] != seq[i-1]+1 {
			t.Fatalf("seq not strictly increasing by one: %v", seq)
		}
	}
	if !strings.Contains(out.String(), `"failure_class":"code"`) || !strings.Contains(out.String(), `"evidence":{"result":"fail"}`) {
		t.Fatalf("missing details:\n%s", out.String())
	}
}

func TestSynthFromSeqKeepsTheResult(t *testing.T) {
	var out bytes.Buffer
	s := NewSynth(&out, "r1", 3)
	s.Finish(View{RunID: "r1", State: "passed", Jobs: []JobState{{Name: "build", State: "passed"}}}, map[string]any{})
	types, seq := seqs(t, out.String())
	if strings.Join(types, ",") != "finished,result" || seq[0] != 3 || seq[1] != 4 {
		t.Fatalf("types %v seq %v", types, seq)
	}
	if line, err := ResultLine("r1", 7, map[string]any{"a": 1}); err != nil || !strings.Contains(string(line), `"seq":7`) || !strings.Contains(string(line), `"type":"result"`) {
		t.Fatalf("%s %v", line, err)
	}
}

func TestTerminal(t *testing.T) {
	for state, want := range map[string]bool{"running": false, "queued": false, "": false, "passed": true, "failed": true, "cancelled": true, "lost": true} {
		if Terminal(state) != want {
			t.Errorf("Terminal(%q) != %v", state, want)
		}
	}
}
