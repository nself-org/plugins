package provision

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestTrustedDynamicProgramsStillRejectSSH(t *testing.T) {
	for _, tc := range []struct {
		name, path, fn, program string
		bad                     bool
	}{
		{"executor argv", "internal/exec/process.go", "runCommandFiles", "program", false},
		{"executor ssh", "internal/exec/process.go", "runCommandFiles", `"ssh"`, true},
		{"untrusted argv", "internal/exec/process.go", "other", "program", true},
		{"sweep runtime", "internal/exec/sweep.go", "removeStaleContainer", "binary", false},
		{"sweep ssh", "internal/exec/sweep.go", "removeStaleContainer", `"ssh"`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "package exec\nimport e \"os/exec\"\nfunc " + tc.fn + "(){ e.Command(" + tc.program + ") }"
			f, err := parser.ParseFile(token.NewFileSet(), tc.path, src, 0)
			if err != nil {
				t.Fatal(err)
			}
			_, bad := inspectFile(f, tc.path, nil)
			if (len(bad) > 0) != tc.bad {
				t.Fatalf("violations %v, want bad %v", bad, tc.bad)
			}
		})
	}
}
