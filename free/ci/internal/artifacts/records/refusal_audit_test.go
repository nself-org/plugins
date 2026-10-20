package records_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestGetRefusalsAreAudited(t *testing.T) {
	for _, tc := range []struct{ name, reason, code string }{
		{"destination", "dest.cwd", "E683"},
		{"corrupt", "digest_mismatch", "E712"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.write("out/app", "payload")
			rec := e.commit("a1", model.ArtifactDecl{Path: "out/app"})[0]
			dest := filepath.Join(e.ws, "dest")
			if tc.name == "destination" {
				dest = e.ws
			} else {
				for _, file := range e.blobFiles() {
					if err := os.WriteFile(file, []byte("PAYLOAD"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			_, _, err := (records.Reader{Ledger: e.st, Blobs: e.blobs}).Get(context.Background(), authz.Viewer{Project: "proj", Role: "member"}, rec.ID, dest, e.ws)
			if err == nil {
				t.Fatal("expected refusal")
			}
			rows, err := e.st.AuditEntries(context.Background(), 0, 1000)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				if row.Action == "artifact.get.refused" && strings.Contains(row.Detail, "code="+tc.code) && strings.Contains(row.Detail, "reason="+tc.reason) {
					return
				}
			}
			t.Fatalf("missing refusal audit: %+v", rows)
		})
	}
}
