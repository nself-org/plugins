package netscope

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The image lock (ADR 0030) and the planner constant must name the same reference, and the lock must list both
// platforms, so the daily image probe covers exactly what the sidecar starts.
func TestProxyImageMatchesTheImageLock(t *testing.T) {
	raw, err := os.ReadFile("../../../../images.json")
	if err != nil {
		t.Fatalf("images.json (plugins root) must be readable: %v", err)
	}
	var lock struct {
		Images map[string]struct {
			Kind      string   `json:"kind"`
			Image     string   `json:"image"`
			Platforms []string `json:"platforms"`
			Owner     string   `json:"owner"`
		} `json:"images"`
	}
	if err := json.Unmarshal(raw, &lock); err != nil {
		t.Fatal(err)
	}
	entry, ok := lock.Images["ci-netproxy"]
	if !ok || entry.Kind != "ci" || entry.Owner != "P7-TRUST-08" {
		t.Fatalf("no ci-netproxy entry owned by P7-TRUST-08: %+v", entry)
	}
	if entry.Image != ProxyImage {
		t.Fatalf("lock %q differs from the constant %q", entry.Image, ProxyImage)
	}
	if strings.Join(entry.Platforms, ",") != "linux/amd64,linux/arm64" {
		t.Errorf("platforms %v", entry.Platforms)
	}
}
