package main

// Purpose: command-level proof that every `nself infra server destroy` refusal
// path leaves the server alone (P7-CANON-12). Core's server_test.go covers the
// missing-backup gate with one case; this file adds each refusal named by the
// port Ticket (no backup flag, snapshot start failure, snapshot action/verify
// failure, primary-IP step failure, missing id, missing token), in both text
// and --json mode, and asserts the recording client saw NO DeleteServer call.
// Inputs: a recording fake that wraps fakeServerClient (server_test.go).
// Outputs: test results. No network: the client is always a fake or the real
// token resolver with an empty environment.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/nself-infra/internal/server"

	"github.com/spf13/cobra"
)

// recordingClient counts calls and injects one failure per field.
type recordingClient struct {
	*fakeServerClient
	deletes, snapshots, ipWrites int
	failSnapshot                 error
	failImage                    error
	actionError                  bool
	failIPList, failIPSet        error
}

func (r *recordingClient) DeleteServer(ctx context.Context, id int64) error {
	r.deletes++
	return r.fakeServerClient.DeleteServer(ctx, id)
}

func (r *recordingClient) CreateSnapshot(ctx context.Context, id int64, d string) (*server.Image, *server.Action, error) {
	r.snapshots++
	if r.failSnapshot != nil {
		return nil, nil, r.failSnapshot
	}
	if r.actionError {
		return &server.Image{ID: 500, Status: "creating"},
			&server.Action{ID: 1, Status: "error", Error: &server.ActionError{Code: "boom", Message: "snapshot failed"}}, nil
	}
	return r.fakeServerClient.CreateSnapshot(ctx, id, d)
}

func (r *recordingClient) GetImage(ctx context.Context, id int64) (*server.Image, error) {
	if r.failImage != nil {
		return nil, r.failImage
	}
	return r.fakeServerClient.GetImage(ctx, id)
}

func (r *recordingClient) ListPrimaryIPs(ctx context.Context, id int64) ([]server.PrimaryIP, error) {
	if r.failIPList != nil {
		return nil, r.failIPList
	}
	return r.fakeServerClient.ListPrimaryIPs(ctx, id)
}

func (r *recordingClient) SetPrimaryIPAutoDelete(ctx context.Context, ipID int64, v bool) error {
	r.ipWrites++
	if r.failIPSet != nil {
		return r.failIPSet
	}
	return r.fakeServerClient.SetPrimaryIPAutoDelete(ctx, ipID, v)
}

func newRecording() *recordingClient {
	fc := newFakeServerClient()
	fc.servers[1] = &server.Server{ID: 1, Name: "web-1"}
	fc.primaryIPs[10] = &server.PrimaryIP{ID: 10, IP: "203.0.113.10", AssigneeID: 1, AutoDelete: true}
	return &recordingClient{fakeServerClient: fc}
}

func runDestroy(t *testing.T, rc *recordingClient, jsonOut bool, flags map[string]string) error {
	t.Helper()
	resetServerFlags()
	withFakeServerClient(t, rc.fakeServerClient)
	old := newServerClient
	newServerClient = func(*cobra.Command) (server.Client, error) { return rc, nil }
	t.Cleanup(func() { newServerClient = old })
	cmd := serverDestroyCmd
	for k, v := range flags {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set --%s: %v", k, err)
		}
	}
	if jsonOut {
		_ = cmd.Flags().Set("json", "true")
	}
	cmd.SetContext(context.Background())
	return runServerDestroy(cmd, nil)
}

func TestServerDestroyRefusals(t *testing.T) {
	boom := errors.New("injected failure")
	cases := []struct {
		name    string
		setup   func(*recordingClient)
		flags   map[string]string
		wantErr string
		noIP    bool // the refusal must happen before any primary-IP write
	}{
		{"no backup flag", nil, map[string]string{"id": "1"}, "destroy refused: no verified backup", true},
		{"release-ip does not waive the gate", nil, map[string]string{"id": "1", "release-ip": "true"}, "destroy refused: no verified backup", true},
		{"missing id", nil, map[string]string{"force-no-backup": "true"}, "server ID is required", true},
		{"snapshot start fails", func(r *recordingClient) { r.failSnapshot = boom }, map[string]string{"id": "1", "snapshot": "true"}, "server NOT deleted", true},
		{"snapshot action errors", func(r *recordingClient) { r.actionError = true }, map[string]string{"id": "1", "snapshot": "true"}, "server NOT deleted", true},
		{"snapshot verify (image poll) fails", func(r *recordingClient) { r.failImage = boom }, map[string]string{"id": "1", "snapshot": "true"}, "server NOT deleted", true},
		{"primary IP list fails", func(r *recordingClient) { r.failIPList = boom }, map[string]string{"id": "1", "force-no-backup": "true"}, "server NOT deleted", true},
		{"primary IP protect fails", func(r *recordingClient) { r.failIPSet = boom }, map[string]string{"id": "1", "force-no-backup": "true"}, "server NOT deleted", false},
		{"snapshot ok but primary IP protect fails", func(r *recordingClient) { r.failIPSet = boom }, map[string]string{"id": "1", "snapshot": "true"}, "server NOT deleted", false},
	}
	for _, tc := range cases {
		for _, jsonOut := range []bool{false, true} {
			name := tc.name
			if jsonOut {
				name += " (--json)"
			}
			t.Run(name, func(t *testing.T) {
				rc := newRecording()
				if tc.setup != nil {
					tc.setup(rc)
				}
				err := runDestroy(t, rc, jsonOut, tc.flags)
				if err == nil {
					t.Fatal("destroy must be refused, got nil error")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err, tc.wantErr)
				}
				if rc.deletes != 0 {
					t.Fatalf("DeleteServer was called %d time(s) on a refused destroy", rc.deletes)
				}
				if _, ok := rc.servers[1]; !ok {
					t.Fatal("server is gone after a refused destroy")
				}
				if tc.noIP && rc.ipWrites != 0 {
					t.Fatalf("primary IP was modified (%d write) before the refusal", rc.ipWrites)
				}
			})
		}
	}
}

// A refused destroy with no backup flag must not even create a snapshot or
// touch an IP: the gate is the very first step.
func TestServerDestroyGateIsFirstStep(t *testing.T) {
	rc := newRecording()
	if err := runDestroy(t, rc, false, map[string]string{"id": "1"}); !errors.Is(err, server.ErrNoBackup) {
		t.Fatalf("want ErrNoBackup, got %v", err)
	}
	if rc.snapshots != 0 || rc.ipWrites != 0 || rc.deletes != 0 {
		t.Fatalf("gate was not first: snapshots=%d ipWrites=%d deletes=%d", rc.snapshots, rc.ipWrites, rc.deletes)
	}
}

// The refusal hint names the plugin's own spelling.
func TestServerDestroyRefusalHint(t *testing.T) {
	rc := newRecording()
	err := runDestroy(t, rc, true, map[string]string{"id": "1"})
	if err == nil || !strings.Contains(err.Error(), "see 'nself infra server destroy --help'") {
		t.Fatalf("hint missing from %v", err)
	}
}

// Without a token the real client factory refuses before any network call.
func TestServerDestroyNoTokenRefused(t *testing.T) {
	resetServerFlags()
	t.Setenv("HETZNER_NSELF_TOKEN", "")
	t.Setenv("HCLOUD_TOKEN", "")
	cmd := serverDestroyCmd
	_ = cmd.Flags().Set("id", "1")
	_ = cmd.Flags().Set("force-no-backup", "true")
	cmd.SetContext(context.Background())
	old := newServerClient
	newServerClient = buildServerClient
	t.Cleanup(func() { newServerClient = old })
	err := runServerDestroy(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "no Hetzner API token found") {
		t.Fatalf("want a token error, got %v", err)
	}
}
