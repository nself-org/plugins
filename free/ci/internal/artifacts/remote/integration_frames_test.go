//go:build integration

package remote

// Purpose: the golden comparison of the frames a node sends (acceptance: only
// protocol v1 message types and fields, nothing local to this Ticket). The
// golden file lists, per message type, the top-level JSON keys of its body.
// UPDATE_GOLDEN=1 rewrites it after a deliberate protocol change.

import (
	"encoding/json"
	"os"
	"sort"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

const framesGolden = "testdata/frames.golden.json"

func frameShapes(frames []protocol.Message) map[string][]string {
	set := map[string]map[string]bool{}
	for _, m := range frames {
		b, err := json.Marshal(m.Body)
		if err != nil {
			panic(err)
		}
		var kv map[string]json.RawMessage
		_ = json.Unmarshal(b, &kv)
		if set[m.Type] == nil {
			set[m.Type] = map[string]bool{}
		}
		for k := range kv {
			set[m.Type][k] = true
		}
	}
	out := map[string][]string{}
	for typ, keys := range set {
		for k := range keys {
			out[typ] = append(out[typ], k)
		}
		sort.Strings(out[typ])
	}
	return out
}

func checkFrames(t *testing.T, frames []protocol.Message) {
	t.Helper()
	got := frameShapes(frames)
	if os.Getenv("UPDATE_GOLDEN") == "1" { // merge: the file also lists the types a short test may not emit (heartbeat, log, ack)
		if raw, err := os.ReadFile(framesGolden); err == nil {
			var old map[string][]string
			_ = json.Unmarshal(raw, &old)
			for typ, keys := range old {
				if _, ok := got[typ]; !ok {
					got[typ] = keys
				}
			}
		}
		b, _ := json.MarshalIndent(got, "", "  ")
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(framesGolden, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(framesGolden)
	if err != nil {
		t.Fatalf("golden frames missing: %v", err)
	}
	var want map[string][]string
	if err = json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	for typ, keys := range got {
		allowed := map[string]bool{}
		for _, k := range want[typ] {
			allowed[k] = true
		}
		if len(want[typ]) == 0 {
			t.Errorf("message type %q is not in the golden set", typ)
		}
		for _, k := range keys {
			if !allowed[k] {
				t.Errorf("message %q carries field %q that the golden set does not have", typ, k)
			}
		}
	}
}
