package main

import (
	"context"
	"crypto/tls"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestAdminCertSubjects(t *testing.T) {
	r := newAdminRig(t)
	t.Setenv("NSELF_CI_HOME", filepath.Dir(r.path))
	for _, tc := range []struct {
		principal string
		self      bool
		want      int
	}{{"alice", true, 0}, {"bob", false, 1}, {"alice", false, 2}} {
		args := []string{"issue", tc.principal, "--json"}
		if tc.self {
			args = append(args, "--self")
		}
		if code := runAdminCert(args, r.env()); code != 0 {
			t.Fatalf("issue: %d %s", code, r.errb.String())
		}
		d := r.data()
		if _, err := tls.X509KeyPair([]byte(d["cert_pem"].(string)), []byte(d["key_pem"].(string))); err != nil {
			t.Fatal(err)
		}
		st, err := store.Open(r.path, store.Options{Clock: r.now})
		if err != nil {
			t.Fatal(err)
		}
		n, err := st.NonSelfPrincipals(context.Background())
		row, rowErr := st.PKICertGet(context.Background(), d["serial"].(string))
		entries, auditErr := st.AuditEntries(context.Background(), 0, 100)
		_ = st.Close()
		if rowErr != nil || row.Kind != "user" || row.NodeID != tc.principal {
			t.Fatalf("certificate identity: %+v %v", row, rowErr)
		}
		if auditErr != nil || len(entries) == 0 || entries[len(entries)-1].Action != "cert.issue" || entries[len(entries)-1].Target != tc.principal {
			t.Fatalf("missing issuance audit: %v", auditErr)
		}
		if err != nil || n != tc.want {
			t.Fatalf("%s self=%v: non-self=%d want %d err=%v", tc.principal, tc.self, n, tc.want, err)
		}
	}
}

func TestAdminCertRejectsInvalidIdentityBeforeIssuing(t *testing.T) {
	for _, args := range [][]string{{"issue", "bob", "--self"}, {"issue", "self"}, {"issue", "BAD"}, {"issue", "alice", "--ttl", "169h"}} {
		r := newAdminRig(t)
		t.Setenv("NSELF_CI_HOME", filepath.Dir(r.path))
		args = append(args, "--json")
		if code := runAdminCert(args, r.env()); code == 0 {
			t.Fatalf("accepted %v", args)
		}
		st, err := store.Open(r.path, store.Options{Clock: r.now})
		if err != nil {
			t.Fatal(err)
		}
		rows, err := st.PKIAuthorities(context.Background())
		_ = st.Close()
		if err != nil || len(rows) != 0 {
			t.Fatalf("invalid request initialized CA: %v %v", rows, err)
		}
	}
}
