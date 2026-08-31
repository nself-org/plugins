// Package ws implements the WebSocket subscribe handler for nself-sync.
//
// Protocol (V04-F04 compliant — JWT NEVER in URL):
//
//  1. Client GETs /sync/subscribe with NO query params or Authorization header.
//  2. Server upgrades to WebSocket.
//  3. Client MUST send an AUTH frame within AuthDeadline (5s):
//       {"type":"auth","token":"<JWT>","cursor":{"hlc_wall_ms":N,"hlc_lamport":M}}
//  4. Server validates the JWT (HS256, NSELF_SYNC_JWT_SECRET).
//     On failure → close code 4001/4002.
//  5. Server LISTENs on np_sync_user_<user_id> and streams matching rows as
//     {"type":"op","data":{...event...}} frames.
//  6. Server sends a ping every HeartbeatInterval; missing pong within
//     PongDeadline closes the connection.
//
// Why the AUTH frame instead of an URL token? URL query strings are logged
// everywhere: web access logs, proxy logs, CDN logs, browser history. A leaked
// access log = a leaked credential. The post-connect AUTH frame keeps the JWT
// in the WebSocket message body, never on the request line.
package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/nself-org/nself-sync/internal/auth"
	"github.com/nself-org/nself-sync/internal/store"
)

// Tunables. Production deployments may override via env if needed.
var (
	// AuthDeadline bounds the wait for the AUTH frame after upgrade.
	AuthDeadline = 5 * time.Second

	// HeartbeatInterval is the cadence of server-initiated pings.
	HeartbeatInterval = 30 * time.Second

	// PongDeadline is the longest the server waits for a pong reply.
	PongDeadline = 10 * time.Second

	// StreamBacklog caps the number of events delivered per fetch.
	StreamBacklog = 500
)

// Close codes (private RFC 6455 4xxx range).
const (
	CloseCodeAuthRequired  = 4001
	CloseCodeAuthInvalid   = 4002
	CloseCodeInternalError = 4500
)

// Frame types exchanged on the wire.
const (
	FrameAuth = "auth"
	FrameOp   = "op"
	FrameAck  = "ack"
	FrameErr  = "error"
)

// AuthFrame is the first message the client must send after upgrade.
type AuthFrame struct {
	Type   string       `json:"type"`
	Token  string       `json:"token"`
	Cursor store.Cursor `json:"cursor"`
}

// OpFrame is the server→client event frame.
type OpFrame struct {
	Type string          `json:"type"`
	Data store.SyncEvent `json:"data"`
}

// AckFrame is sent after successful auth.
type AckFrame struct {
	Type   string `json:"type"`
	UserID string `json:"user_id"`
}

// ErrFrame conveys errors before close.
type ErrFrame struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Listener is the minimal interface ws.Handler depends on for streaming.
// Implemented by *store.Listener in production; mocked in tests.
type Listener interface {
	Listen(ctx context.Context, channel string) error
	WaitForNotification(ctx context.Context) (*Notification, error)
	EventsSince(ctx context.Context, userID string, cur store.Cursor, limit int) ([]store.SyncEvent, error)
	Close(ctx context.Context) error
}

// Notification is a transport-agnostic NOTIFY payload.
type Notification struct {
	Channel string
	Payload string
}

// ListenerFactory builds a Listener per WS connection.
type ListenerFactory func(ctx context.Context) (Listener, error)

// Handler returns the http.HandlerFunc for GET /sync/subscribe.
func Handler(secret []byte, factory ListenerFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: false,
			OriginPatterns:     allowedOrigins(),
		})
		if err != nil {
			log.Printf("ws: upgrade failed: %v", err)
			return
		}
		// Default close is non-error; explicit close codes are set on the
		// failure paths below.
		defer c.Close(websocket.StatusNormalClosure, "")

		ctx := r.Context()
		if err := handleConn(ctx, c, secret, factory); err != nil {
			// Logged at info level — auth failures and disconnects are normal.
			log.Printf("ws: %v", err)
		}
	}
}

// handleConn drives the per-connection lifecycle.
func handleConn(
	ctx context.Context,
	c *websocket.Conn,
	secret []byte,
	factory ListenerFactory,
) error {
	// 1) Read the AUTH frame within AuthDeadline. We race a real read against
	// a timer so we can issue a clean close frame on timeout (nhooyr aborts
	// the connection if we just cancel the read context).
	//
	// The inner read uses a derived authCtx that is cancelled on every exit
	// path (timeout, parent ctx cancel, or successful receive). Combined with
	// a buffered result channel and a select on authCtx.Done() in the sender,
	// this prevents the read goroutine from leaking if the parent context is
	// cancelled before either the AUTH frame arrives or the 5s timeout fires
	// (SIEGE finding V13-S3).
	type authResult struct {
		af  AuthFrame
		err error
	}
	// authCtx uses AuthDeadline + 500ms grace so the goroutine's wsjson.Read
	// context never fires concurrently with time.After(AuthDeadline). Without
	// the buffer, both timers fire at the same moment: nhooyr cancels the
	// underlying connection before the explicit c.Close(4001) frame is sent,
	// causing the client to observe close code -1 instead of 4001 (flaky test,
	// V13-S3 variant).
	authCtx, authCancel := context.WithTimeout(ctx, AuthDeadline+500*time.Millisecond)
	defer authCancel()
	authResCh := make(chan authResult, 1) // buffered: sender can exit even if receiver is gone
	go func() {
		var af AuthFrame
		err := wsjson.Read(authCtx, c, &af)
		select {
		case authResCh <- authResult{af: af, err: err}:
		case <-authCtx.Done():
			// Receiver already gone; drop the result rather than block forever.
		}
	}()

	var af AuthFrame
	select {
	case <-time.After(AuthDeadline):
		_ = c.Close(CloseCodeAuthRequired, "auth frame required within "+AuthDeadline.String())
		return errors.New("auth frame deadline")
	case <-ctx.Done():
		// Parent context cancelled. authCancel (deferred) will release the
		// inner wsjson.Read promptly, and the buffered channel + select in
		// the sender guarantee the goroutine exits.
		return ctx.Err()
	case res := <-authResCh:
		if res.err != nil {
			_ = c.Close(CloseCodeAuthRequired, "auth frame read failed")
			return fmt.Errorf("await auth frame: %w", res.err)
		}
		af = res.af
	}
	if af.Type != FrameAuth || af.Token == "" {
		_ = c.Close(CloseCodeAuthRequired, "first frame must be auth")
		return errors.New("invalid auth frame")
	}

	// 2) Verify the JWT.
	claims, err := auth.Verify(af.Token, secret)
	if err != nil {
		_ = c.Close(CloseCodeAuthInvalid, "invalid token")
		return fmt.Errorf("auth verify: %w", err)
	}

	// 3) Ack the client. We never echo the token.
	if err := wsjson.Write(ctx, c, AckFrame{Type: FrameAck, UserID: claims.UserID}); err != nil {
		return fmt.Errorf("write ack: %w", err)
	}

	// 4) Build a dedicated listener for this connection.
	listener, err := factory(ctx)
	if err != nil {
		_ = c.Close(CloseCodeInternalError, "subscribe init failed")
		return fmt.Errorf("listener factory: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = listener.Close(closeCtx)
	}()

	channel := store.ChannelForUser(claims.UserID)
	if err := listener.Listen(ctx, channel); err != nil {
		_ = c.Close(CloseCodeInternalError, "listen failed")
		return fmt.Errorf("LISTEN %s: %w", channel, err)
	}

	// 5) Catch-up before entering the notification loop.
	startCursor := store.Cursor{WallMs: af.Cursor.WallMs, Lamport: af.Cursor.Lamport}
	delivered, err := streamSince(ctx, c, listener, claims.UserID, startCursor)
	if err != nil {
		return fmt.Errorf("initial catch-up: %w", err)
	}
	if delivered.WallMs != 0 || delivered.Lamport != 0 {
		startCursor = delivered
	}

	// 6) Heartbeat + notify loop.
	return runLoop(ctx, c, listener, claims.UserID, startCursor)
}

// runLoop multiplexes heartbeat, NOTIFY pump, and disconnect detection.
func runLoop(
	ctx context.Context,
	c *websocket.Conn,
	listener Listener,
	userID string,
	cursor store.Cursor,
) error {
	hb := time.NewTicker(HeartbeatInterval)
	defer hb.Stop()

	// Read pump: drains client→server frames so the websocket library can
	// process control frames (pongs) and so we detect disconnects promptly.
	readErr := make(chan error, 1)
	go func() {
		for {
			_, _, err := c.Read(ctx)
			if err != nil {
				readErr <- err
				return
			}
		}
	}()

	// Notification pump.
	notifyCh := make(chan *Notification, 8)
	notifyErr := make(chan error, 1)
	go func() {
		for {
			n, err := listener.WaitForNotification(ctx)
			if err != nil {
				notifyErr <- err
				return
			}
			if n == nil {
				continue
			}
			select {
			case notifyCh <- n:
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case err := <-readErr:
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				return nil
			}
			return fmt.Errorf("read pump: %w", err)

		case err := <-notifyErr:
			_ = c.Close(CloseCodeInternalError, "notify failure")
			return fmt.Errorf("notify pump: %w", err)

		case <-hb.C:
			pingCtx, cancel := context.WithTimeout(ctx, PongDeadline)
			err := c.Ping(pingCtx)
			cancel()
			if err != nil {
				_ = c.Close(websocket.StatusPolicyViolation, "ping timeout")
				return fmt.Errorf("heartbeat ping: %w", err)
			}

		case <-notifyCh:
			// Fetch events newer than running cursor.
			events, err := listener.EventsSince(ctx, userID, cursor, StreamBacklog)
			if err != nil {
				_ = c.Close(CloseCodeInternalError, "fetch failed")
				return fmt.Errorf("EventsSince after NOTIFY: %w", err)
			}
			for i := range events {
				if err := wsjson.Write(ctx, c, OpFrame{Type: FrameOp, Data: events[i]}); err != nil {
					return fmt.Errorf("write op: %w", err)
				}
				cursor = store.Cursor{
					WallMs:  events[i].HLCWallMs,
					Lamport: events[i].HLCLamport,
				}
			}
		}
	}
}

// streamSince delivers events strictly newer than `from`. Returns the cursor
// after the last delivered event (or `from` if nothing was delivered).
func streamSince(
	ctx context.Context,
	c *websocket.Conn,
	listener Listener,
	userID string,
	from store.Cursor,
) (store.Cursor, error) {
	events, err := listener.EventsSince(ctx, userID, from, StreamBacklog)
	if err != nil {
		return from, err
	}
	cursor := from
	for i := range events {
		if err := wsjson.Write(ctx, c, OpFrame{Type: FrameOp, Data: events[i]}); err != nil {
			return cursor, err
		}
		cursor = store.Cursor{
			WallMs:  events[i].HLCWallMs,
			Lamport: events[i].HLCLamport,
		}
	}
	return cursor, nil
}

// allowedOrigins reads NSELF_SYNC_WS_ORIGINS (comma-separated). Empty list
// means nhooyr enforces same-origin.
func allowedOrigins() []string {
	v := os.Getenv("NSELF_SYNC_WS_ORIGINS")
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// NotificationFromJSON parses a NOTIFY payload (the np_sync_events row trigger
// encodes the inserted row id as JSON). Exposed for tests.
func NotificationFromJSON(raw string) (*Notification, error) {
	if raw == "" {
		return &Notification{}, nil
	}
	var n Notification
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		return nil, err
	}
	return &n, nil
}
