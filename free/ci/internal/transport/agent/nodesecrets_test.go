package agent

// Purpose: tests of node-side secret resolution: the local secrets frame carries values the coordinator never
//          sent, resolution is asynchronous, a failure sends nothing, and a coordinator-sent secrets frame is
//          dropped.
// Inputs:  a scripted inner carrier and resolver. Constraints: bounded waits.

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
)

type scripted struct {
	in     chan protocol.Message
	mu     sync.Mutex
	sent   []protocol.Message
	closed bool
}

func newScripted() *scripted { return &scripted{in: make(chan protocol.Message, 16)} }

func (s *scripted) Send(m protocol.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, m)
	return nil
}
func (s *scripted) Recv() (protocol.Message, error) {
	m, ok := <-s.in
	if !ok {
		return protocol.Message{}, io.EOF
	}
	return m, nil
}
func (s *scripted) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.in)
	}
	return nil
}

func leaseWithSecrets(refs ...string) protocol.Message {
	l := protoLease("l1", "a1", 3)
	l.CompatMode = protocol.Compat15
	for _, r := range refs {
		l.Secrets = append(l.Secrets, protocol.SecretRef{Ref: r, Class: model.SecretClass(strings.SplitN(r, "/", 2)[0])})
	}
	ep := l.Epoch
	return protocol.Message{V: 1, Type: "lease", LeaseID: l.LeaseID, Epoch: &ep, Body: &l}
}

func recvWithin(t *testing.T, f Frames, d time.Duration) protocol.Message {
	t.Helper()
	ch := make(chan protocol.Message, 1)
	go func() { m, _ := f.Recv(); ch <- m }()
	select {
	case m := <-ch:
		return m
	case <-time.After(d):
		t.Fatal("nothing arrived")
		return protocol.Message{}
	}
}

func TestNodeSecretsResolveLocallyAndSendAFrameToTheSession(t *testing.T) {
	inner := newScripted()
	var gotAttempt string
	var gotRefs []string
	f := WithNodeSecrets(inner, func(_ context.Context, attempt string, refs []string) (map[string]string, error) {
		gotAttempt, gotRefs = attempt, refs
		return map[string]string{"API_TOKEN": "value-1", "DB_PASS": "value-2"}, nil
	}, nil)
	defer f.Close()
	inner.in <- leaseWithSecrets("project/API_TOKEN", "environment/staging/DB_PASS")
	first := recvWithin(t, f, 2*time.Second)
	if first.Type != "lease" {
		t.Fatalf("first message %s, want the lease unchanged and first", first.Type)
	}
	m := recvWithin(t, f, 2*time.Second)
	b, ok := m.Body.(*protocol.Secrets)
	if m.Type != "secrets" || !ok || m.LeaseID != "l1" || m.Epoch == nil || *m.Epoch != 3 || b.LeaseID != "l1" || len(b.Values) != 2 {
		t.Fatalf("secrets message = %+v", m)
	}
	if b.Values[0].Ref != "project/API_TOKEN" || b.Values[0].Value != "value-1" || b.Values[1].Ref != "environment/staging/DB_PASS" || b.Values[1].Value != "value-2" {
		t.Fatalf("values = %+v", b.Values)
	}
	if gotAttempt != "a1" || len(gotRefs) != 2 || gotRefs[0] != "project/API_TOKEN" {
		t.Fatalf("resolver saw %q %v", gotAttempt, gotRefs)
	}
	if len(inner.sent) != 0 {
		t.Fatal("the wrapper sent something upstream on its own")
	}
}

func TestNodeSecretsLeaseWithoutRefsStartsNothing(t *testing.T) {
	inner := newScripted()
	called := make(chan struct{}, 1)
	f := WithNodeSecrets(inner, func(context.Context, string, []string) (map[string]string, error) {
		called <- struct{}{}
		return nil, nil
	}, nil)
	defer f.Close()
	inner.in <- leaseWithSecrets()
	_ = recvWithin(t, f, 2*time.Second)
	select {
	case <-called:
		t.Fatal("the resolver ran for a lease without secrets")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestNodeSecretsFailureSendsNothingAndLogsOnlyTheCode(t *testing.T) {
	inner := newScripted()
	var mu sync.Mutex
	var notes []string
	f := WithNodeSecrets(inner, func(context.Context, string, []string) (map[string]string, error) {
		return nil, &secrets.Error{Code: "E677", Message: "node is not eligible: the-secret-value-leaked-here"}
	}, func(format string, a ...any) {
		mu.Lock()
		notes = append(notes, format+" "+strings.Join(toStrings(a), " "))
		mu.Unlock()
	})
	defer f.Close()
	inner.in <- leaseWithSecrets("project/API_TOKEN")
	_ = recvWithin(t, f, 2*time.Second)
	eventually(t, 2*time.Second, "the failure to be logged", func() bool { mu.Lock(); defer mu.Unlock(); return len(notes) == 1 })
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(notes[0], "E677") || strings.Contains(notes[0], "leaked") {
		t.Fatalf("log line %q must carry the code only", notes[0])
	}
	inner.in <- protocol.Message{V: 1, Type: "drain", Body: &protocol.Drain{Deadline: "x"}}
	if m := recvWithin(t, f, 2*time.Second); m.Type != "drain" {
		t.Fatalf("after a failed resolution the next message is %s, want drain (no secrets frame)", m.Type)
	}
}

func toStrings(a []any) []string {
	out := make([]string, len(a))
	for i, v := range a {
		switch x := v.(type) {
		case error:
			out[i] = codeOf(x)
		case string:
			out[i] = x
		}
	}
	return out
}

func TestNodeSecretsCodeOfPlainErrorIsE676(t *testing.T) {
	if codeOf(errors.New("boom")) != "E676" || codeOf(&secrets.Error{Code: "E675"}) != "E675" {
		t.Fatal("codeOf")
	}
}

func TestNodeSecretsDropsAValueFrameFromTheCoordinator(t *testing.T) {
	inner := newScripted()
	f := WithNodeSecrets(inner, func(context.Context, string, []string) (map[string]string, error) { return nil, nil }, nil)
	defer f.Close()
	inner.in <- protocol.Message{V: 1, Type: "secrets", LeaseID: "l1", Body: &protocol.Secrets{LeaseID: "l1", Values: []protocol.SecretValue{{Ref: "project/X", Value: "from-the-coordinator"}}}}
	inner.in <- protocol.Message{V: 1, Type: "drain", Body: &protocol.Drain{Deadline: "x"}}
	if m := recvWithin(t, f, 2*time.Second); m.Type != "drain" {
		t.Fatalf("got %s, want the secrets frame dropped", m.Type)
	}
}

func TestNodeSecretsIgnoresRefsItCannotParse(t *testing.T) {
	inner := newScripted()
	f := WithNodeSecrets(inner, func(context.Context, string, []string) (map[string]string, error) {
		return map[string]string{"GOOD": "v"}, nil
	}, nil)
	defer f.Close()
	inner.in <- leaseWithSecrets("project/GOOD", "nonsense")
	_ = recvWithin(t, f, 2*time.Second)
	m := recvWithin(t, f, 2*time.Second)
	b := m.Body.(*protocol.Secrets)
	if len(b.Values) != 1 || b.Values[0].Ref != "project/GOOD" {
		t.Fatalf("values = %+v", b.Values)
	}
}

func TestNodeSecretsForwardsSendAndEndsOnClose(t *testing.T) {
	inner := newScripted()
	f := WithNodeSecrets(inner, func(context.Context, string, []string) (map[string]string, error) { return nil, nil }, nil)
	if err := f.Send(protocol.Message{Type: "ack"}); err != nil || len(inner.sent) != 1 {
		t.Fatalf("send: %v %d", err, len(inner.sent))
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if _, err := f.Recv(); err == nil {
		t.Fatal("Recv after Close must end")
	}
}

func TestNodeSecretsPassesInnerErrors(t *testing.T) {
	inner := newScripted()
	f := WithNodeSecrets(inner, nil, nil)
	defer f.Close()
	_ = inner.Close()
	if _, err := f.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("err = %v", err)
	}
}
