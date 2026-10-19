package coordclient

// Purpose: tests of the TLS client: the server must be the pinned coordinator, refusals carry E664 reason
// keys end to end (node certificate, expired and unknown tokens, a role below the route), and a token
// file is read only when only its owner can touch it.
// Inputs:  the simulated coordinator of rig_test.go. Outputs: pass or fail.
// Constraints: real TLS 1.3 on 127.0.0.1.

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/roles"
)

func TestCoordinatorClientServerIdentity(t *testing.T) {
	s := newSim(t)
	user := s.userFiles("mia")
	t.Run("no pinned root", func(t *testing.T) {
		c := user
		c.CA = ""
		if _, err := New(s.target(c)); !reasonIs(err, clientapi.ReasonServerIdentity) {
			t.Fatalf("a client without ci.coordinator_ca must refuse: %v", err)
		}
	})
	t.Run("another coordinator's root", func(t *testing.T) {
		other := newSim(t)
		c := user
		c.CA = other.rootFile()
		cl, err := New(s.target(c))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = cl.Get(context.Background(), "latest"); !reasonIs(err, clientapi.ReasonServerIdentity) {
			t.Fatalf("a server that chains to another root must be refused: %v", err)
		}
	})
	t.Run("a leaf that is not a coordinator's", func(t *testing.T) {
		node := s.nodeFiles("node-9")
		pair, err := tls.LoadX509KeyPair(node.Cert, node.Key)
		if err != nil {
			t.Fatal(err)
		}
		fake := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("{}")) }))
		fake.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS13}
		fake.StartTLS()
		defer fake.Close()
		cl, err := New(Target{Kind: Remote, URL: fake.URL, Creds: user})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = cl.Get(context.Background(), "latest"); !reasonIs(err, clientapi.ReasonServerIdentity) {
			t.Fatalf("a node certificate served as the coordinator must be refused: %v", err)
		}
	})
	t.Run("plain http", func(t *testing.T) {
		if _, err := New(Target{Kind: Remote, URL: "http://127.0.0.1:3865", Creds: user}); !reasonIs(err, clientapi.ReasonPlainHTTP) {
			t.Fatalf("%v", err)
		}
	})
}

func TestCoordinatorClientRefusals(t *testing.T) {
	s := newSim(t)
	ctx := context.Background()
	root := s.rootFile()
	t.Run("node certificate", func(t *testing.T) {
		_, _, err := s.client(s.nodeFiles("node-1")).Get(ctx, "latest")
		if !reasonIs(err, clientapi.ReasonNodeKind) {
			t.Fatalf("a node certificate on the user API must be E664 node_kind: %v", err)
		}
	})
	t.Run("expired token", func(t *testing.T) {
		tf := s.tokenFile("eve", roles.Member, time.Minute, "acme/app")
		s.svc.Now = func() time.Time { return time.Now().Add(2 * time.Minute) }
		defer func() { s.svc.Now = time.Now }()
		_, _, err := s.client(Creds{CA: root, TokenFile: tf}).Get(ctx, "latest")
		if !reasonIs(err, clientapi.ReasonTokenExpired) {
			t.Fatalf("an expired token must be E664 token_expired: %v", err)
		}
	})
	t.Run("unknown token", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "tok")
		_ = os.WriteFile(p, []byte("never-issued"), 0o600)
		if _, _, err := s.client(Creds{CA: root, TokenFile: p}).Get(ctx, "latest"); !reasonIs(err, clientapi.ReasonTokenUnknown) {
			t.Fatalf("%v", err)
		}
	})
	t.Run("a role below the route", func(t *testing.T) {
		tf := s.tokenFile("mia", roles.Member, time.Hour, "acme/other")
		cl := s.client(Creds{CA: root, TokenFile: tf})
		if _, err := cl.Submit(ctx, clientapi.SubmitRequest{Project: "acme/app", Revision: testRev}); !reasonIs(err, clientapi.ReasonProject) {
			t.Fatalf("a token scoped to another project must be E664 project: %v", err)
		}
	})
	t.Run("the token never shows in an error", func(t *testing.T) {
		tf := s.tokenFile("zoe", roles.Member, time.Minute, "acme/app")
		raw, _ := os.ReadFile(tf)
		s.svc.Now = func() time.Time { return time.Now().Add(2 * time.Minute) }
		defer func() { s.svc.Now = time.Now }()
		_, _, err := s.client(Creds{CA: root, TokenFile: tf}).Get(ctx, "latest")
		if err == nil || len(raw) < 20 || contains(err.Error(), string(raw[:20])) {
			t.Fatalf("error text leaks the token: %v", err)
		}
	})
}

func contains(a, b string) bool { return len(b) > 0 && len(a) >= len(b) && indexOf(a, b) >= 0 }

func indexOf(a, b string) int {
	for i := 0; i+len(b) <= len(a); i++ {
		if a[i:i+len(b)] == b {
			return i
		}
	}
	return -1
}

func TestCoordinatorClientTokenFileGuards(t *testing.T) {
	s := newSim(t)
	root := s.rootFile()
	open := filepath.Join(t.TempDir(), "open")
	_ = os.WriteFile(open, []byte("tok"), 0o644)
	if _, err := New(s.target(Creds{CA: root, TokenFile: open})); !reasonIs(err, clientapi.ReasonCredentialFile) {
		t.Fatalf("a token file others can read must be refused: %v", err)
	}
	empty := filepath.Join(t.TempDir(), "empty")
	_ = os.WriteFile(empty, []byte("\n"), 0o600)
	if _, err := New(s.target(Creds{CA: root, TokenFile: empty})); !reasonIs(err, clientapi.ReasonCredentialFile) {
		t.Fatalf("an empty token file: %v", err)
	}
	if _, err := New(s.target(Creds{CA: root})); !reasonIs(err, clientapi.ReasonNoCredential) {
		t.Fatalf("no certificate and no token: %v", err)
	}
}
