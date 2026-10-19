package session

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

const transcriptDir = "../../protocol/testdata/transcripts"

// agentOrigin says which side of a golden transcript wrote a line: the agent
// wrote hello, result and its acks; the scripted coordinator wrote the rest.
func agentOrigin(name, kind string) bool {
	switch kind {
	case "hello", "result", "log", "artifact-ref", "cache-ref":
		return true
	case "ack":
		return name == "cancel" || name == "drain"
	}
	return false
}

// rawSigner returns two bytes whose unpadded base64 is "sig", the value in the golden.
type rawSigner struct{}

func (rawSigner) KeyID() string               { return "k1" }
func (rawSigner) Sign([]byte) ([]byte, error) { return []byte{0xb2, 0x28}, nil }

// replay runs the agent against the coordinator lines of a P7-NODE-02 golden
// transcript and returns the bytes the agent wrote and the golden's agent lines.
func replay(t *testing.T, name string, resume *Resume, wait string) (got, want []byte, err error) {
	t.Helper()
	raw, rerr := os.ReadFile(filepath.Join(transcriptDir, name+".ndjson"))
	if rerr != nil {
		t.Fatal(rerr)
	}
	var coordLines, agentLines [][]byte
	for _, line := range bytes.SplitAfter(raw, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var head struct{ Type string }
		if e := json.Unmarshal(line, &head); e != nil {
			t.Fatal(e)
		}
		if agentOrigin(name, head.Type) {
			agentLines = append(agentLines, line)
		} else {
			coordLines = append(coordLines, line)
		}
	}
	state := t.TempDir()
	table, e := slots.Open(state, 2)
	if e != nil {
		t.Fatal(e)
	}
	car := newFakeCarrier()
	s, e := New(Config{Carrier: car, Key: fakeKey{}, Slots: table, Runner: okRunner(`{"schema":"ci.evidence/v1"}`),
		Coordinator: Coordinator{ID: "c1"}, StateDir: state, Clock: newManualClock(), Hooks: []JobHook{}, Signer: rawSigner{}, Resume: resume,
		Facts: Facts{AgentVersion: "1.5.0", MachineID: "m1", CapabilityDigest: "sha", AgeRecipient: ptr("age1"), HostKeyFingerprints: []string{}}})
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()
	dec := protocol.NewDecoder(bytes.NewReader(bytes.Join(coordLines, nil)), testPolicy)
	var enc bytes.Buffer
	agentEnc := protocol.NewEncoder(&enc)
	if resume != nil {
		agentEnc, _ = protocol.NewResumedEncoder(&enc, resume.SendSeq, resume.SendSeq)
	}
	for range coordLines {
		m, e := dec.Decode()
		if e != nil {
			t.Fatal(e)
		}
		if m.Type == "welcome" {
			// the golden's hello comes first; the scripted coordinator answers it
			if e := agentEnc.Encode(<-outOf(car, t)); e != nil {
				t.Fatal(e)
			}
		}
		car.in <- m
		if m.Type == "lease" {
			if e := agentEnc.Encode(<-outOf(car, t)); e != nil { // the result
				t.Fatal(e)
			}
		}
		if m.Type == "cancel" || m.Type == "drain" {
			if e := agentEnc.Encode(<-outOf(car, t)); e != nil { // the ack
				t.Fatal(e)
			}
		}
	}
	if wait == "end" {
		err = <-done
	} else {
		_ = car.Close()
		err = <-done
	}
	return enc.Bytes(), bytes.Join(agentLines, nil), err
}

func outOf(c *fakeCarrier, t *testing.T) chan protocol.Message {
	ch := make(chan protocol.Message, 1)
	go func() {
		for m := range c.out {
			if m.Type == "heartbeat" {
				continue
			}
			ch <- m
			return
		}
	}()
	return ch
}

func ptr[T any](v T) *T { return &v }

// TestTranscriptReplay proves the acceptance line: serve replays the
// happy-path, cancel and drain transcripts of P7-NODE-02 byte-identically
// against a scripted coordinator. The agent's frames are re-encoded with the
// protocol codec and compared with the golden's agent lines.
func TestTranscriptReplay(t *testing.T) {
	cases := []struct {
		name   string
		resume *Resume
		wait   string
	}{
		{"happy", nil, "close"},
		{"cancel", &Resume{Welcome: testWelcome}, "close"},
		{"drain", &Resume{Welcome: testWelcome}, "end"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, want, err := replay(t, c.name, c.resume, c.wait)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) == 0 || !bytes.Equal(got, want) {
				t.Fatalf("agent frames differ from the golden\n got: %s\nwant: %s", got, want)
			}
		})
	}
}
