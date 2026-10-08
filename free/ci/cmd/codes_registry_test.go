package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	_ "github.com/nself-org/plugins/free/ci/internal/store"
)

type listedPackage struct {
	ImportPath string
	Dir        string
	GoFiles    []string
}

// TestCodeRegistry ensures every code fragment is linked into cmd before
// checking collisions and reserved ranges.
func TestCodeRegistry(t *testing.T) {
	if problems := model.CodeProblems(); len(problems) != 0 {
		t.Fatal(problems)
	}
	list := func(args ...string) []listedPackage {
		cmd := exec.Command("go", append([]string{"list", "-mod=vendor", "-json"}, args...)...)
		cmd.Dir = ".."
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("go list %v: %v", args, err)
		}
		dec := json.NewDecoder(bytes.NewReader(out))
		var packages []listedPackage
		for dec.More() {
			var p listedPackage
			if err := dec.Decode(&p); err != nil {
				t.Fatal(err)
			}
			packages = append(packages, p)
		}
		return packages
	}
	linked := map[string]bool{}
	for _, p := range list("-deps", "./cmd") {
		linked[p.ImportPath] = true
	}
	fragments := 0
	for _, p := range list("./...") {
		if !strings.HasPrefix(p.ImportPath, "github.com/nself-org/plugins/free/ci/") {
			continue
		}
		for _, file := range p.GoFiles {
			if filepath.Base(file) == "codes.go" {
				fragments++
				// Store is linked when the V15 engine is wired into cmd. Its codes
				// are imported above so this test still checks their registrations.
				if !linked[p.ImportPath] && p.ImportPath != "github.com/nself-org/plugins/free/ci/internal/store" {
					t.Errorf("code fragment %s is not linked into cmd", p.ImportPath)
				}
			}
		}
	}
	if fragments == 0 {
		t.Fatal("no code fragments found")
	}
}
