package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeValues(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	gen := filepath.Join(dir, ".nself", "generated", "k8s")
	if err := os.MkdirAll(gen, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gen, "values.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gen, "secrets.yaml"), []byte("secrets: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestInstalledLinePrintsIngressHosts: the success line lists the generated
// ingress hosts, never the --domain value, and prints no URL when none is routed.
func TestInstalledLinePrintsIngressHosts(t *testing.T) {
	dir := writeValues(t, "ingress:\n  rules:\n    - {name: h, host: api.example.test, service: hasura, port: 8080, scheme: http}\n")
	got := installedLine(dir)
	if !strings.Contains(got, "https://api.example.test") {
		t.Errorf("line = %q, want the ingress host URL", got)
	}
	none := installedLine(writeValues(t, "ingress:\n  rules: []\n"))
	if strings.Contains(none, "https://") || !strings.Contains(none, "no ingress host") {
		t.Errorf("no rules: line = %q, want no URL", none)
	}
}

// TestFlagHelpSaysNotConsumed: the flag help of --domain and --plugins and the
// upgrade text state that the chart does not consume them yet (D-0311).
func TestFlagHelpSaysNotConsumed(t *testing.T) {
	for _, name := range []string{"domain", "plugins"} {
		f := installCmd.Flags().Lookup(name)
		if f == nil || !strings.Contains(f.Usage, "D-0311") {
			t.Errorf("install --%s help does not mention D-0311: %v", name, f)
		}
	}
	if !strings.Contains(installCmd.Long, "Not consumed yet") || !strings.Contains(upgradeCmd.Long, "D-0311") {
		t.Error("install or upgrade Long text is missing the not-consumed note")
	}
}
