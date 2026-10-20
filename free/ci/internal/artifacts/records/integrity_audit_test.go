package records_test

import (
	"context"
	"os"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestCASIntegrityRefusalsAreAuditedByCaller(t *testing.T) {
	for _, op := range []string{"get", "commit"} {
		t.Run(op, func(t *testing.T) {
			e := newEnv(t)
			e.write("out/app", "payload")
			f, ds := e.attempt("a1", "test", model.ArtifactDecl{Path: "out/app"})
			if _, err := e.stage("a1", f, ds); err != nil {
				t.Fatal(err)
			}
			e.finalizing("a1")
			var recs []records.Record
			if op == "get" {
				var err error
				recs, err = records.CommitAndVerify(t.Context(), e.st, e.blobs, "a1", 3, e.pol)
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, file := range e.blobFiles() {
				if err := os.WriteFile(file, []byte("PAYLOAD"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var err error
			if op == "commit" {
				_, err = records.CommitAndVerify(t.Context(), e.st, e.blobs, "a1", 3, e.pol)
			} else {
				_, _, err = (records.Reader{Ledger: e.st, Blobs: e.blobs}).Get(t.Context(), authz.Viewer{Project: "proj", Role: "member"}, recs[0].ID, e.ws+"/dest", e.ws)
			}
			if err == nil {
				t.Fatal("expected corrupt blob refusal")
			}
			rows, err := e.st.AuditEntries(context.Background(), 0, 1000)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rows {
				if r.Action == "blob.integrity.refused" && r.Detail == "code=E710 reason=digest_mismatch" {
					return
				}
			}
			t.Fatalf("missing E710 audit: %+v", rows)
		})
	}
}
