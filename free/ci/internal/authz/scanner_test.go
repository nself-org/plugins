package authz_test

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// The redactor is the production ValueScanner (P7-TRUST-19, test-only edge).
var _ authz.ValueScanner = (*redact.Redactor)(nil)

func TestValueScanner_RedactorFindsRegisteredValue(t *testing.T) {
	r := redact.New()
	if err := r.Register([]byte("synthetic-secret-value-0001")); err != nil {
		t.Fatal(err)
	}
	var s authz.ValueScanner = r
	blob := []byte("header synthetic-secret-value-0001 trailer")
	got := s.Scan(blob)
	if len(got) != 1 || got[0] != (authz.Match{Offset: 7, Len: 27}) {
		t.Fatalf("matches %+v", got)
	}
	if authz.Clean(s, blob) {
		t.Fatal("blob with a registered value reported clean")
	}
	if !authz.Clean(s, []byte("nothing here")) {
		t.Fatal("clean blob reported dirty")
	}
}

func TestClean_NilScannerIsNotClean(t *testing.T) {
	if authz.Clean(nil, []byte("x")) {
		t.Fatal("no scanner must not pass")
	}
}
