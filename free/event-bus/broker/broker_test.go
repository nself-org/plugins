// Purpose: Baseline tests for the event-bus broker package.
// Inputs: Message struct, Broker interface definition, type constants.
// Outputs: Struct field access, interface method set verification at compile time.
// Constraints: No live NATS/Kafka broker required; interface and type tests only.
package broker

import "testing"

// TestBuild verifies the broker package compiles and the Message type is accessible.
func TestBuild(t *testing.T) {
	t.Log("event-bus broker package compiled OK")
}

// TestMessageStruct verifies Message fields are accessible.
func TestMessageStruct(t *testing.T) {
	called := false
	msg := Message{
		Subject: "test.subject",
		Payload: []byte("hello"),
		Ack: func() error {
			called = true
			return nil
		},
		Nak: func() error { return nil },
	}
	if msg.Subject != "test.subject" {
		t.Errorf("Message.Subject = %q, want %q", msg.Subject, "test.subject")
	}
	if string(msg.Payload) != "hello" {
		t.Errorf("Message.Payload = %q, want %q", msg.Payload, "hello")
	}
	if err := msg.Ack(); err != nil {
		t.Errorf("Message.Ack() error: %v", err)
	}
	if !called {
		t.Error("Message.Ack() should have invoked the provided func")
	}
}
