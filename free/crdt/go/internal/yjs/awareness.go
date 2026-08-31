// Package yjs provides the server-side Yjs y-websocket protocol handler.
package yjs

import (
	"sync"
)

// awarenessMsg is an in-memory awareness message (cursor positions, user presence).
// Awareness messages are NEVER persisted — in-memory only per spec.
type awarenessMsg struct {
	ClientID string
	Data     []byte
}

// AwarenessHub manages in-memory awareness pub/sub per document.
// Each document room has a set of subscribers. When a client broadcasts
// awareness, all other clients in the same room receive it.
type AwarenessHub struct {
	mu    sync.RWMutex
	rooms map[string]map[string]chan []byte // docID -> clientID -> channel
}

// NewAwarenessHub creates a new hub.
func NewAwarenessHub() *AwarenessHub {
	return &AwarenessHub{
		rooms: make(map[string]map[string]chan []byte),
	}
}

// Subscribe registers a client channel for a document room.
// Returns a channel on which awareness messages from other clients are delivered.
func (h *AwarenessHub) Subscribe(docID, clientID string) chan []byte {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.rooms[docID] == nil {
		h.rooms[docID] = make(map[string]chan []byte)
	}
	h.rooms[docID][clientID] = ch
	return ch
}

// Unsubscribe removes a client from a document room and closes its channel.
func (h *AwarenessHub) Unsubscribe(docID, clientID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if room, ok := h.rooms[docID]; ok {
		if ch, ok := room[clientID]; ok {
			close(ch)
			delete(room, clientID)
		}
		if len(room) == 0 {
			delete(h.rooms, docID)
		}
	}
}

// Broadcast sends an awareness message to all other clients in the document room.
func (h *AwarenessHub) Broadcast(docID, senderClientID string, data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	room, ok := h.rooms[docID]
	if !ok {
		return
	}
	for clientID, ch := range room {
		if clientID == senderClientID {
			continue
		}
		// Non-blocking send: if the client's buffer is full, drop the message.
		// Awareness is best-effort; stale cursor positions are harmless.
		select {
		case ch <- data:
		default:
		}
	}
}
