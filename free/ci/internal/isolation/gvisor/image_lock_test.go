package gvisor

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The image lock (ADR 0030) and the probe constant must name the same digest, and the lock must
// list both platforms, so the daily image probe covers exactly what the profile starts.
func TestProbeImageMatchesTheImageLock(t *testing.T) {
	raw, err := os.ReadFile("../../../../../images.json")
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
	entry, ok := lock.Images["ci-gvisor-probe"]
	if !ok || entry.Kind != "ci" || entry.Owner != "P7-TRUST-05" {
		t.Fatalf("no ci-gvisor-probe entry owned by P7-TRUST-05: %+v", entry)
	}
	_, probeDigest, _ := strings.Cut(ProbeImage, "@")
	_, lockDigest, _ := strings.Cut(entry.Image, "@")
	if probeDigest == "" || probeDigest != lockDigest {
		t.Fatalf("digest differs: probe %q lock %q", probeDigest, lockDigest)
	}
	if !strings.HasPrefix(entry.Image, strings.SplitN(ProbeImage, "@", 2)[0]+":") {
		t.Errorf("lock repository differs from the probe's: %s vs %s", entry.Image, ProbeImage)
	}
	if strings.Join(entry.Platforms, ",") != "linux/amd64,linux/arm64" {
		t.Errorf("platforms %v", entry.Platforms)
	}
}
