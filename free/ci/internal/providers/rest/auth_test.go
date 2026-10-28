package rest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

func TestGHTokenFailureClasses(t *testing.T) {
	for _, tc := range []struct {
		name, script, code string
		deadline           bool
	}{
		{name: "missing", code: "E700"},
		{name: "login", script: "#!/bin/sh\nexit 1\n", code: "E700"},
		{name: "start", script: "#!/missing/interpreter\n", code: "E704"},
		{name: "timeout", script: "#!/bin/sh\nwhile :; do :; done\n", code: "E704", deadline: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PATH", dir)
			t.Setenv("NSELF_CI_HOME", t.TempDir())
			if tc.script != "" {
				if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(tc.script), 0700); err != nil {
					t.Fatal(err)
				}
			}
			ctx := context.Background()
			if tc.deadline {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			_, err := GHToken("github.com")(ctx)
			var pe *providers.Error
			if !errors.As(err, &pe) || pe.Code != tc.code {
				t.Fatalf("got %v, want %s", err, tc.code)
			}
			if tc.code == "E704" && (pe.Class != providers.Transient || pe.Reason != "gh.timeout") {
				t.Fatalf("transient error: %+v", pe)
			}
		})
	}
}
