package coordclient

// Purpose: GetRaw, the generic authenticated GET other Tickets read their registered routes through
// (P7-CACHE-17): it carries the credential, returns any status unread, refuses a path outside the user
// API and maps a transport failure to the usual reason key.
// Constraints: the simulated TEAM coordinator of rig_test.go; 127.0.0.1 only.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/roles"
)

func TestGetRawCarriesTheCredentialAndReturnsAnyStatus(t *testing.T) {
	s := newSim(t)
	tok := s.tokenFile("u-member", roles.Member, time.Hour, "acme/app")
	c := s.client(Creds{CA: s.rootFile(), TokenFile: tok})
	resp, err := c.GetRaw(context.Background(), clientapi.PathPing)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("ping: %v %v", resp, err)
	}
	_ = resp.Body.Close()

	// an answer that is not 2xx is handed back with its own body, not turned into an error
	resp, err = c.GetRaw(context.Background(), "/v1/runs/no-such-run")
	if err != nil {
		t.Fatalf("a 404 must not be an error: %v", err)
	}
	var b clientapi.Body
	_ = json.NewDecoder(resp.Body).Decode(&b)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound || b.Code != clientapi.CodeUserAPI || b.Reason != clientapi.ReasonNotFound {
		t.Fatalf("status %d body %+v", resp.StatusCode, b)
	}
}

func TestGetRawRefusesPathsOutsideTheUserAPI(t *testing.T) {
	s := newSim(t)
	c := s.client(Creds{CA: s.rootFile(), TokenFile: s.tokenFile("u", roles.Member, time.Hour, "acme/app")})
	for _, p := range []string{"", "v1/ping", "/healthz", "//evil.example/v1/x", "/v1/x\r\nHost: evil", "/v1/a b", `/v1/a\b`} {
		if resp, err := c.GetRaw(context.Background(), p); err == nil {
			_ = resp.Body.Close()
			t.Errorf("path %q accepted", p)
		} else if !reasonIs(err, clientapi.ReasonBadRequest) {
			t.Errorf("path %q: %v", p, err)
		}
	}
}

func TestGetRawMapsTransportFailure(t *testing.T) {
	s := newSim(t)
	c := s.client(Creds{CA: s.rootFile(), TokenFile: s.tokenFile("u", roles.Member, time.Hour, "acme/app")})
	s.srv.Close()
	if resp, err := c.GetRaw(context.Background(), clientapi.PathPing); err == nil {
		_ = resp.Body.Close()
		t.Fatal("a closed coordinator answered")
	} else if !reasonIs(err, clientapi.ReasonTransport) {
		t.Fatalf("transport failure mapped to %v", err)
	}
}
