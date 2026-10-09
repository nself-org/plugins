package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func frame(kind string, seq uint64, body any) Message {
	return Message{V: 1, Type: kind, Seq: seq, Body: body}
}
func goldenCases() map[string][]Message {
	set := "age1"
	hello := Hello{Versions: []int{1}, AgentVersion: "1.5.0", AgeRecipient: &set, NodeKey: NodeKey{Alg: "ed25519", KeyID: "k1", Public: "pub"}, CapabilityDigest: "sha", Identity: Identity{MachineID: "m1", HostKeyFingerprints: []string{}}, CompatModeSupported: []CompatMode{Compat14, Compat15}, Slots: Slots{Total: 2}}
	nullHello := hello
	nullHello.AgeRecipient = nil
	pin := "different"
	answer, _ := AcceptHello(hello, &pin, []int{1}, "1.4.0", "s1", 10000, 30000)
	lease := Lease{LeaseID: "l1", AttemptID: "a1", Epoch: 2, Token: "opaque", CompatMode: Compat15, Secrets: []SecretRef{}, ExpiresAt: "2026-10-08T20:00:00Z", Cache: &LeaseCache{Namespace: "n", Access: "rw", Restore: []CacheRestore{}, Save: []CacheSave{}}, RunnerCredential: &RunnerCredential{Kind: "github-jitconfig", Value: "YQ==", ExpiresAt: "2026-10-08T20:00:00Z"}}
	lm := frame("lease", 3, lease)
	e := uint64(2)
	lm.Epoch = &e
	lm.LeaseID = "l1"
	rm := frame("result", 4, Result{LeaseID: "l1", Epoch: 2, Evidence: json.RawMessage(`{"schema":"ci.evidence/v1"}`), Signature: Signature{KeyID: "k1", Value: "sig"}})
	rm.Epoch = &e
	rm.LeaseID = "l1"
	return map[string][]Message{
		"happy":               {frame("hello", 1, hello), frame("welcome", 2, Welcome{Version: 1, MinAgentVersion: "1.4.0", SessionID: "s1", HeartbeatMS: 10000, AgentTTLMS: 30000}), lm, rm},
		"nminus1-agent":       {frame("hello", 1, Hello{Versions: []int{1}, AgentVersion: "1.4.0", AgeRecipient: nil, NodeKey: hello.NodeKey, CapabilityDigest: "sha", Identity: hello.Identity, CompatModeSupported: []CompatMode{Compat14}, Slots: hello.Slots}), frame("welcome", 2, Welcome{Version: 1, MinAgentVersion: "1.4.0", SessionID: "s-old", HeartbeatMS: 10000, AgentTTLMS: 30000})},
		"nminus1-coordinator": {frame("hello", 1, hello), frame("welcome", 2, Welcome{Version: 1, MinAgentVersion: "1.4.0", SessionID: "s-new", HeartbeatMS: 10000, AgentTTLMS: 30000})},
		"cancel":              {frame("cancel", 1, Cancel{LeaseID: "l1", Reason: "operator"}), frame("ack", 2, Ack{AckSeq: 1, LeaseID: "l1"})},
		"reject":              {frame("hello", 1, nullHello), frame("reject", 2, Reject{Code: CodeVersion, Reason: "no_common_version", MinAgentVersion: "1.4.0"})},
		"drain":               {frame("drain", 1, Drain{Deadline: "2026-10-08T20:00:00Z"}), frame("ack", 2, Ack{AckSeq: 1})},
		"resume":              {frame("resume", 1, Resume{LastSeq: 44}), frame("offset", 2, Offset{Stream: "artifact", SHA256: "abc", Offset: 10})},
		"ssh-loss":            {frame("log", 1, Log{LeaseID: "l1", LineSeq: 1, Stream: "stdout", Text: "started"}), frame("reject", 2, Reject{Code: CodeInvalid, Reason: "ssh_session_lost"})},
		"cache":               {frame("digest", 1, Digest{Stream: "cache", SHA256: "abc", Size: 2}), frame("offset", 2, Offset{Stream: "cache", SHA256: "abc", Offset: 0}), frame("chunk", 3, Chunk{Stream: "cache", Data: "YQ==", Digest: "abc", Final: true}), frame("cache-ref", 4, CacheRef{LeaseID: "l1", Spec: "s", Key: "k", Blobs: []CacheBlob{{Path: "p", SHA256: "abc", Size: 1}}})},
		"age-mismatch":        {frame("hello", 1, hello), frame("reject", 2, answer.(Reject))},
	}
}

func fenceGolden(m *Message) {
	if !leaseScoped(m.Type) {
		return
	}
	e := uint64(2)
	m.Epoch = &e
	m.LeaseID = "l1"
}

func TestGoldenTranscripts(t *testing.T) {
	for name, msgs := range goldenCases() {
		t.Run(name, func(t *testing.T) {
			var encoded bytes.Buffer
			enc := [2]*Encoder{NewEncoder(&encoded), NewEncoder(&encoded)}
			seq := [2]uint64{}
			for i, m := range msgs {
				dir := transcriptDirection(name, i, m.Type)
				seq[dir]++
				m.Seq = seq[dir]
				fenceGolden(&m)
				if err := enc[dir].Encode(m); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join("testdata", "transcripts", name+".ndjson")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, encoded.Bytes(), 0644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(want, encoded.Bytes()) {
				t.Fatal("golden changed")
			}
			var round bytes.Buffer
			re := [2]*Encoder{NewEncoder(&round), NewEncoder(&round)}
			seq = [2]uint64{}
			lines := bytes.SplitAfter(want, []byte("\n"))
			for i, m := range msgs {
				dir := transcriptDirection(name, i, m.Type)
				dec, err := NewResumedDecoder(bytes.NewReader(lines[i]), CarrierPolicy{Carrier: "A", Personal: true, OperatorOwned: true}, seq[dir], seq[dir])
				if err != nil {
					t.Fatal(err)
				}
				got, err := dec.Decode()
				if err != nil {
					t.Fatal(err)
				}
				seq[dir] = got.Seq
				if err := re[dir].Encode(got); err != nil {
					t.Fatal(err)
				}
			}
			if !bytes.Equal(want, round.Bytes()) {
				t.Fatal("roundtrip changed")
			}
		})
	}
}

func transcriptDirection(name string, index int, kind string) int {
	switch kind {
	case "hello", "result", "log", "artifact-ref", "cache-ref":
		return 0
	case "welcome", "reject", "lease", "cancel", "drain":
		return 1
	case "ack":
		if name == "cancel" || name == "drain" {
			return 0
		}
	}
	if name == "cache" && index == 3 {
		return 0
	}
	return 1
}

func TestStrictSeqCarrierAndPreamble(t *testing.T) {
	cases := []string{`{"v":1,"type":"ack","seq":2,"body":{"ack_seq":1}}` + "\n", `{"v":1,"type":"ack","seq":1,"body":{"ack_seq":1,"surprise":1}}` + "\n", `{"v":1,"type":"chunk","seq":1,"body":{"stream":"cache","offset":0,"data":"YQ==","final":true}}` + "\n"}
	for _, raw := range cases {
		if _, err := NewDecoder(strings.NewReader(raw), CarrierPolicy{}).Decode(); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, bad := range []uint64{1, 3} {
		raw := `{"v":1,"type":"ack","seq":1,"body":{"ack_seq":1}}` + "\n" +
			`{"v":1,"type":"ack","seq":` + strconv.FormatUint(bad, 10) + `,"body":{"ack_seq":1}}` + "\n"
		d := NewDecoder(strings.NewReader(raw), CarrierPolicy{})
		if _, err := d.Decode(); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Decode(); err == nil {
			t.Fatalf("accepted repeated or gapped seq %d", bad)
		}
	}
	secret := `{"v":1,"type":"secrets","seq":1,"body":{"lease_id":"l","values":[{"ref":"project/x","value":"s"}]}}` + "\n"
	for _, p := range []CarrierPolicy{{Carrier: "B", Personal: true, OperatorOwned: true}, {Carrier: "A", Personal: false, OperatorOwned: true}, {Carrier: "A", Personal: true, OperatorOwned: false}} {
		if _, err := NewDecoder(strings.NewReader(secret), p).Decode(); err == nil {
			t.Fatal("accepted unsafe secret")
		}
	}
	if _, err := NewDecoder(strings.NewReader(secret), CarrierPolicy{Carrier: "A", Personal: true, OperatorOwned: true}).Decode(); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{strings.Repeat("n", MaxShellNoise+1) + Preamble, "noise\n"} {
		if _, err := SkipPreamble(strings.NewReader(raw)); err == nil {
			t.Fatal("accepted shell noise")
		}
	}
	r, err := SkipPreamble(strings.NewReader(strings.Repeat("n", MaxShellNoise-1) + "\n" + Preamble + cases[0]))
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewDecoder(r, CarrierPolicy{}).Decode()
	if err == nil {
		t.Fatal("expected seq rejection after preamble")
	}
}

func TestNegotiationAndAgePin(t *testing.T) {
	if v, r := Negotiate([]int{0}, []int{1}, "1.5.0", "1.4.0"); v != 0 || r == nil || r.Code != CodeVersion || r.MinAgentVersion != "1.4.0" {
		t.Fatal(v, r)
	}
	for _, a := range []string{"1.4.0", "1.5.0"} {
		if v, r := Negotiate([]int{1}, []int{1}, a, "1.4.0"); v != 1 || r != nil {
			t.Fatal(v, r)
		}
	}
	a, b := "old", "new"
	if r := CheckAgeRecipient(Hello{AgeRecipient: &b}, &a); r == nil || r.Code != "E660" {
		t.Fatal(r)
	}
}

func FuzzDecode(f *testing.F) {
	f.Add([]byte(`{"v":1,"type":"ack","seq":1,"body":{"ack_seq":1}}` + "\n"))
	f.Add([]byte(`{"v":1,"type":"ack","seq":2,"body":{"ack_seq":1}}` + "\n"))
	f.Add([]byte(`{"v":1,"type":"ack","seq":1,"body":{"ack_seq":1}}` + "\n" + `{"v":1,"type":"ack","seq":1,"body":{"ack_seq":1}}` + "\n"))
	f.Add(append([]byte(strings.Repeat("x", MaxFrame+1)+"\n"), []byte(`{"v":1,"type":"ack","seq":1,"body":{"ack_seq":1}}`+"\n")...))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 3*MaxFrame {
			data = data[:3*MaxFrame]
		}
		d := NewDecoder(bytes.NewReader(data), CarrierPolicy{Carrier: "B"})
		var previous uint64
		var first error
		for i := 0; i < 4; i++ {
			m, err := d.Decode()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				if first != nil && err != first {
					t.Fatalf("poison changed: %v to %v", first, err)
				}
				first = err
				continue
			}
			if first != nil {
				t.Fatal("resynchronized after error")
			}
			if m.Seq != previous+1 {
				t.Fatalf("accepted seq %d after %d", m.Seq, previous)
			}
			previous = m.Seq
		}
	})
}

func TestDecodeAllocationBound(t *testing.T) {
	data := []byte(strings.Repeat("x", MaxFrame+1) + "\n")
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < 10; i++ {
		_, _ = NewDecoder(bytes.NewReader(data), CarrierPolicy{}).Decode()
	}
	runtime.ReadMemStats(&after)
	if after.TotalAlloc-before.TotalAlloc > 30*MaxFrame {
		t.Fatalf("allocated %d bytes for ten oversized frames", after.TotalAlloc-before.TotalAlloc)
	}
}
