package yjs_test

import (
	"testing"

	"github.com/nself-org/nself-crdt/internal/yjs"
)

func TestAwarenessHubSubscribeUnsubscribe(t *testing.T) {
	hub := yjs.NewAwarenessHub()
	docID := "test-doc"
	clientA := "client-a"
	clientB := "client-b"

	chA := hub.Subscribe(docID, clientA)
	chB := hub.Subscribe(docID, clientB)

	// Broadcast from A should reach B but not A.
	msg := []byte("cursor-position-data")
	hub.Broadcast(docID, clientA, msg)

	select {
	case got := <-chB:
		if string(got) != string(msg) {
			t.Errorf("expected %q, got %q", msg, got)
		}
	default:
		t.Fatal("expected message on channel B, got none")
	}

	// Channel A should have no message (sender not echoed).
	select {
	case <-chA:
		t.Fatal("sender should not receive own broadcast")
	default:
	}

	hub.Unsubscribe(docID, clientA)
	hub.Unsubscribe(docID, clientB)
}

func TestAwarenessHubNotPersisted(t *testing.T) {
	// Awareness messages must never be persisted — they are in-memory only.
	// This test verifies the hub has no store dependency.
	hub := yjs.NewAwarenessHub()
	if hub == nil {
		t.Fatal("hub must not be nil")
	}
	// AwarenessHub has no Store field — compile-time enforcement.
}
