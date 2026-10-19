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
	s.Finish(View{RunID: "r1", State: "failed", Jobs: []JobState{{Name: "test", State: "failed", FailureClass: "code"}, {Name: "build", State: "passed"}}},
		map[string]any{"result": "fail"})
	types, seq := seqs(t, out.String())
	want := []string{"queued", "started", "check_result", "started", "check_result", "finished", "result"}
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

// A client that saw a live stream and reconnects after the run ended must get the same
// records at the same seqs, whatever it observed before.
func TestSynthResumeIsConsistent(t *testing.T) {
	done := View{RunID: "r1", State: "passed", Jobs: []JobState{{Name: "b", State: "passed"}, {Name: "a", State: "passed"}}}
	var live, replay bytes.Buffer
	l := NewSynth(&live, "r1", 0)
	l.Observe(View{RunID: "r1", State: "running", Jobs: []JobState{{Name: "a", State: "running"}, {Name: "b", State: "queued"}}})
	l.Finish(done, map[string]any{})
	NewSynth(&replay, "r1", 0).Finish(done, map[string]any{})
	lt, ls := seqs(t, live.String())
	rt, rs := seqs(t, replay.String())
	if strings.Join(lt, ",") != strings.Join(rt, ",") || len(ls) != len(rs) {
		t.Fatalf("live %v %v, replay %v %v", lt, ls, rt, rs)
	}
	var resumed bytes.Buffer
	NewSynth(&resumed, "r1", 3).Finish(done, map[string]any{})
	types, seq := seqs(t, resumed.String())
	if seq[0] != 3 || strings.Join(types, ",") != strings.Join(rt[2:], ",") {
		t.Fatalf("resume at 3: %v %v, want the tail of %v", types, seq, rt)
	}
}

func TestSynthFromSeqKeepsTheResult(t *testing.T) {
	var out bytes.Buffer
	s := NewSynth(&out, "r1", 5)
	s.Finish(View{RunID: "r1", State: "passed", Jobs: []JobState{{Name: "build", State: "passed"}}}, map[string]any{})
	// queued 1, started 2, check_result 3, finished 4, result 5: from 5 keeps only the result
	types, seq := seqs(t, out.String())
	if strings.Join(types, ",") != "result" || seq[0] != 5 {
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
