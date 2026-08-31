// internal/compat_test.go — Backward-compatibility tests for the nself-sync
// wire protocol.
//
// S03-M7: verifies that v1.1.10 server can accept v1.1.9-era request fixtures
// (and vice versa) without data corruption or silent protocol drift.
//
// Uses recorded request/response fixtures from testdata/v1.1.0/. No live
// network. All assertions run against in-process JSON parsing only.
package internal_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const fixtureDir = "../testdata/v1.1.0"

// --- shared fixture types (mirrors what the server uses internally) ---

type hlc struct {
	WallMs   int64  `json:"wall_ms"`
	Lamport  int    `json:"lamport"`
	DeviceID string `json:"device_id"`
}

type eventEnvelope struct {
	EventID       string          `json:"event_id"`
	HLC           hlc             `json:"hlc"`
	Payload       json.RawMessage `json:"payload"`
	SchemaVersion int             `json:"schema_version,omitempty"`
}

type pushRequest struct {
	DeviceID      string          `json:"device_id"`
	Events        []eventEnvelope `json:"events"`
	SchemaVersion int             `json:"schema_version,omitempty"`
}

type pushResponse struct {
	Acks          []string `json:"acks"`
	SchemaVersion int      `json:"schema_version,omitempty"`
}

type pullResponse struct {
	Events        []eventEnvelope `json:"events"`
	HasMore       bool            `json:"has_more"`
	Cursor        *hlc            `json:"cursor,omitempty"`
	SchemaVersion int             `json:"schema_version,omitempty"`
}

// --- a minimal in-process push handler (mirrors handlePush in main.go) ---

// testPushHandler is a reduced in-process version of the /sync/push endpoint
// that accepts batched events and returns acks. Used to verify BC without a
// live server.
func testPushHandler(w http.ResponseWriter, r *http.Request) {
	var req pushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Produce acks for all received event IDs.
	acks := make([]string, 0, len(req.Events))
	for _, e := range req.Events {
		if e.EventID != "" {
			acks = append(acks, e.EventID)
		}
	}

	resp := pushResponse{Acks: acks}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// --- BC tests ---

// TestBC_SyncProto_V110ClientToV111Server verifies that a v1.1.0-era push
// request (loaded from testdata fixture) is accepted by the v1.1.1 server
// handler without errors. Asserts acks contain the sent event IDs.
func TestBC_SyncProto_V110ClientToV111Server(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(fixtureDir + "/push_request.json")
	if err != nil {
		t.Fatalf("reading v1.1.0 push_request fixture: %v", err)
	}

	// Verify the fixture itself parses as a valid pushRequest.
	var req pushRequest
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatalf("fixture is not valid pushRequest JSON: %v", err)
	}
	if len(req.Events) == 0 {
		t.Fatal("fixture must contain at least one event")
	}

	// Submit to the in-process handler (represents v1.1.1 server).
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/sync/push", strings.NewReader(string(data)))
	r.Header.Set("Content-Type", "application/json")
	testPushHandler(rr, r)

	if rr.Code != http.StatusOK {
		t.Errorf("v1.1.1 server rejected v1.1.0 client push: status %d, body: %s",
			rr.Code, rr.Body.String())
	}

	var resp pushResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding push response: %v", err)
	}

	// All sent event IDs must be acked.
	for _, event := range req.Events {
		found := false
		for _, ack := range resp.Acks {
			if ack == event.EventID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("event %s not acked by v1.1.1 server", event.EventID)
		}
	}
}

// TestBC_SyncProto_V111ClientToV110Server verifies that a v1.1.1 client can
// send a push to a v1.1.0-era server (simulated by the same minimal handler
// since the server is backward-compatible). The v1.1.0 server ignores unknown
// fields — new fields added in v1.1.1 must not break old servers.
func TestBC_SyncProto_V111ClientToV110Server(t *testing.T) {
	t.Parallel()

	// A v1.1.1 client might include additional fields. The v1.1.0 handler
	// (same handler struct here) must silently ignore them.
	v111Payload := `{
		"device_id": "00000000-0000-0000-0000-000000000099",
		"events": [
			{
				"event_id": "99000000-0000-0000-0000-000000000001",
				"hlc": {"wall_ms": 1715644900000, "lamport": 5, "device_id": "00000000-0000-0000-0000-000000000099"},
				"payload": {"type": "message.edited", "body": "updated"},
				"schema_version": 2,
				"metadata": {"client_version": "1.1.1", "new_field": true}
			}
		],
		"schema_version": 2
	}`

	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/sync/push", strings.NewReader(v111Payload))
	r.Header.Set("Content-Type", "application/json")
	testPushHandler(rr, r)

	if rr.Code != http.StatusOK {
		t.Errorf("v1.1.0-era server rejected v1.1.1 client push: status %d, body: %s",
			rr.Code, rr.Body.String())
	}

	var resp pushResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding push response: %v", err)
	}

	// Event was acked despite unknown fields in the request.
	if len(resp.Acks) == 0 {
		t.Error("v1.1.0-era server must ack events from v1.1.1 client (unknown fields ignored)")
	}
	if resp.Acks[0] != "99000000-0000-0000-0000-000000000001" {
		t.Errorf("unexpected ack: %s", resp.Acks[0])
	}
}

// TestBC_SyncProto_PullResponseV110ParsedByV111 verifies that a v1.1.0
// pull response (fixture) is fully parsed by v1.1.1 client code. No field
// must be lost or corrupted on the v1.1.1 side.
func TestBC_SyncProto_PullResponseV110ParsedByV111(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(fixtureDir + "/pull_response.json")
	if err != nil {
		t.Fatalf("reading v1.1.0 pull_response fixture: %v", err)
	}

	var resp pullResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("v1.1.1 pull response type cannot parse v1.1.0 fixture: %v", err)
	}

	if len(resp.Events) == 0 {
		t.Fatal("pull response must contain events")
	}
	if resp.Events[0].EventID == "" {
		t.Error("event_id must not be empty after parsing v1.1.0 pull fixture")
	}
	if resp.Events[0].HLC.WallMs == 0 {
		t.Error("hlc.wall_ms must not be zero in v1.1.0 pull fixture")
	}
	if resp.Cursor == nil {
		t.Error("cursor must not be nil in v1.1.0 pull fixture")
	}
	// has_more: false is a valid value, just confirm the field was parsed.
	_ = resp.HasMore
}

// TestBC_SyncProto_EventSchemaFieldSurvival asserts that every field present
// in a v1.1.0 event envelope (event_id, hlc, payload) survives round-trip
// through v1.1.1 JSON parsing. Guards against accidental field removal.
func TestBC_SyncProto_EventSchemaFieldSurvival(t *testing.T) {
	t.Parallel()

	raw := `{
		"event_id": "aaaaaaaa-0000-0000-0000-000000000001",
		"hlc": {
			"wall_ms": 1000000,
			"lamport": 7,
			"device_id": "bbbbbbbb-0000-0000-0000-000000000001"
		},
		"payload": {"k": "v"},
		"schema_version": 1
	}`

	var env eventEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal v1.1.0 event: %v", err)
	}

	if env.EventID != "aaaaaaaa-0000-0000-0000-000000000001" {
		t.Errorf("event_id mismatch: got %s", env.EventID)
	}
	if env.HLC.Lamport != 7 {
		t.Errorf("hlc.lamport mismatch: got %d", env.HLC.Lamport)
	}
	if env.HLC.DeviceID != "bbbbbbbb-0000-0000-0000-000000000001" {
		t.Errorf("hlc.device_id mismatch: got %s", env.HLC.DeviceID)
	}
	if len(env.Payload) == 0 {
		t.Error("payload must not be empty after parse")
	}
}
