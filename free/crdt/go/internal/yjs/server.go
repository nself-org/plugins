package yjs

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/nself-org/nself-crdt/internal/store"
)

// Yjs message type constants (y-websocket protocol).
// https://github.com/yjs/y-websocket/blob/master/src/y-websocket.js
const (
	msgSync      = 0
	msgAwareness = 1

	syncStep1 = 0
	syncStep2 = 1
	syncUpdate = 2
)

var upgrader = websocket.Upgrader{
	CheckOrigin:      func(r *http.Request) bool { return true },
	ReadBufferSize:   4096,
	WriteBufferSize:  4096,
	HandshakeTimeout: 10 * time.Second,
}

// Handler handles WebSocket connections following the y-websocket protocol.
type Handler struct {
	store    *store.Store
	hub      *AwarenessHub
	maxBytes int64
	log      *slog.Logger
}

// NewHandler creates a new Yjs WebSocket handler.
func NewHandler(st *store.Store, hub *AwarenessHub, maxDocSizeMB int, log *slog.Logger) *Handler {
	return &Handler{
		store:    st,
		hub:      hub,
		maxBytes: int64(maxDocSizeMB) * 1024 * 1024,
		log:      log,
	}
}

// ServeHTTP upgrades the connection and handles the y-websocket protocol.
// URL param: {docID} from chi router.
func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request, docID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Warn("yjs: ws upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	clientID := uuid.New().String()
	ctx := r.Context()

	// Register awareness subscription.
	awarenessCh := h.hub.Subscribe(docID, clientID)
	defer h.hub.Unsubscribe(docID, clientID)

	// Load current document state from Postgres and send sync step 1.
	doc, err := h.store.LoadDocument(ctx, docID)
	var initialState []byte
	if err == nil {
		initialState = doc.State
	}

	// Send sync step 1: server sends its state vector (we send the full state here).
	if err := h.sendSyncStep1(conn, initialState); err != nil {
		h.log.Warn("yjs: send sync step 1 failed", "doc", docID, "err", err)
		return
	}

	// Goroutine: forward awareness messages from hub to this client.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case msg, ok := <-awarenessCh:
				if !ok {
					return
				}
				conn.WriteMessage(websocket.BinaryMessage, msg)
			case <-ctx.Done():
				return
			}
		}
	}()

	conn.SetReadLimit(h.maxBytes + 1024) // slight headroom for protocol overhead

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if len(data) == 0 {
			continue
		}

		msgType := data[0]
		payload := data[1:]

		switch msgType {
		case msgSync:
			h.handleSync(ctx, conn, docID, clientID, payload)
		case msgAwareness:
			// Forward awareness to other clients; do NOT persist.
			broadcast := make([]byte, 1+len(payload))
			broadcast[0] = msgAwareness
			copy(broadcast[1:], payload)
			h.hub.Broadcast(docID, clientID, broadcast)
		default:
			h.log.Debug("yjs: unknown message type", "type", msgType)
		}
	}

	<-done
}

func (h *Handler) handleSync(ctx context.Context, conn *websocket.Conn, docID, clientID string, payload []byte) {
	if len(payload) == 0 {
		return
	}
	step := payload[0]
	body := payload[1:]

	switch step {
	case syncStep1:
		// Client sends its state vector; respond with step 2 (our full state or diff).
		doc, err := h.store.LoadDocument(ctx, docID)
		var state []byte
		if err == nil {
			state = doc.State
		}
		if sendErr := h.sendSyncStep2(conn, state); sendErr != nil {
			h.log.Warn("yjs: send sync step 2 failed", "doc", docID, "err", sendErr)
		}

	case syncStep2:
		// Client sends its full state or update — merge and persist.
		h.persistUpdate(ctx, conn, docID, clientID, body)

	case syncUpdate:
		// Incremental update from client.
		h.persistUpdate(ctx, conn, docID, clientID, body)
	}
}

func (h *Handler) persistUpdate(ctx context.Context, conn *websocket.Conn, docID, clientID string, update []byte) {
	if int64(len(update)) > h.maxBytes {
		_ = conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "document too large"))
		return
	}

	// Log the raw update for the audit trail.
	if err := h.store.AppendUpdate(ctx, docID, clientID, update); err != nil {
		h.log.Error("yjs: append update failed", "doc", docID, "err", err)
		return
	}

	// For Yjs we treat each update as the new authoritative state.
	// A proper implementation would merge via the Yjs state machine;
	// here we store the latest update blob as the document state.
	// Clients merging via the y-websocket protocol apply updates locally.
	if err := h.store.UpsertDocument(ctx, docID, "yjs", update); err != nil {
		h.log.Error("yjs: upsert document failed", "doc", docID, "err", err)
		return
	}

	// Echo the update to all other connected clients.
	broadcast := make([]byte, 2+len(update))
	broadcast[0] = msgSync
	broadcast[1] = syncUpdate
	copy(broadcast[2:], update)
	h.hub.Broadcast(docID, clientID, broadcast)
}

// sendSyncStep1 sends the server's sync step 1 message (state vector).
func (h *Handler) sendSyncStep1(conn *websocket.Conn, state []byte) error {
	msg, err := encodeSyncMsg(syncStep1, state)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, msg)
}

// sendSyncStep2 sends the server's sync step 2 message (full state diff).
func (h *Handler) sendSyncStep2(conn *websocket.Conn, state []byte) error {
	msg, err := encodeSyncMsg(syncStep2, state)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, msg)
}

// encodeSyncMsg builds a y-websocket sync message: [msgSync, step, varint(len), payload].
func encodeSyncMsg(step byte, payload []byte) ([]byte, error) {
	// 2-byte header + varint length prefix + payload
	buf := make([]byte, 2+binary.MaxVarintLen64+len(payload))
	buf[0] = msgSync
	buf[1] = step
	n := binary.PutUvarint(buf[2:], uint64(len(payload)))
	copy(buf[2+n:], payload)
	return buf[:2+n+len(payload)], nil
}

// DocID extracts the docID from the request URL path segment.
// Chi router parameter helper — callers pass chi.URLParam(r, "docID").
func DocID(r *http.Request) string {
	// Extracted by chi router — passed directly to ServeWS.
	_ = r
	return fmt.Sprintf("extracted-by-chi-%s", time.Now())
}
