//go:build darwin && macosleg

package probe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUnacceptedXcodeLicense runs the actual local probe against an executable
// xcodebuild stub. No host license setting is changed.
func TestUnacceptedXcodeLicense(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "calls")
	stub := "#!/bin/sh\necho \"$*\" >> '" + marker + "'\necho 'You have not agreed to the Xcode license agreements' >&2\nexit 69\n"
	if err := os.WriteFile(filepath.Join(dir, "xcodebuild"), []byte(stub), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	const id = "01J00000000000000000000000"
	p := LocalProber{Registry: setupProbe(t, id)}
	n, err := p.Probe(ForceRefresh(context.Background()), id)
	if err != nil {
		t.Fatal(err)
	}
	if calls, err := os.ReadFile(marker); err != nil || !strings.Contains(string(calls), "-version") {
		t.Fatalf("probe did not execute stub xcodebuild -version: %q, %v", calls, err)
	}
	if fact := n.Capability.Tools.Toolchains["xcode"]; fact.Value != nil || fact.Confidence != "unknown" {
		t.Fatalf("unaccepted license advertised Xcode: %+v", fact)
	}
}
