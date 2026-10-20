package run

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestArtifactCollectionRefusalsAreAudited(t *testing.T) {
	for _, tc := range []struct{ name, script, path, code, reason string }{
		{"missing", "echo built", "out/app", "E712", "missing"},
		{"symlink", "ln -s /etc/passwd output", "output", "E683", "link.absolute"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := artRig(t, map[string]model.Job{"build": artJob(tc.script, model.ArtifactDecl{Path: tc.path})})
			res := r.run(t)
			if o := outcome(res, "build"); o.State != StateFailed || o.Code != tc.code {
				t.Fatalf("outcome: %+v", o)
			}
			rows, err := r.store(t).AuditEntries(context.Background(), 0, 1000)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				if row.Action == "artifact.collect.refused" && strings.Contains(row.Detail, "code="+tc.code) && strings.Contains(row.Detail, "reason="+tc.reason) {
					return
				}
			}
			t.Fatalf("missing collection audit: %+v", rows)
		})
	}
}
