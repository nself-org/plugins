package demand_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers/github/demand"
)

// TestPinnedImageMatchesTheImageLockFragment: the digest the handler runs (toolchain pin) is the digest
// images.d/ci-actions-runner.json locks, both are an index digest, and the pin is never a tag.
func TestPinnedImageMatchesTheImageLockFragment(t *testing.T) {
	ref, err := demand.PinnedImage()
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^ghcr\.io/actions/actions-runner@sha256:[0-9a-f]{64}$`).MatchString(ref) {
		t.Fatalf("pinned ref %q", ref)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "..", "images.d", "ci-actions-runner.json"))
	if err != nil {
		t.Fatal(err)
	}
	var frag struct {
		Name, Kind, Image, Owner string
		Platforms                []string
	}
	if err = json.Unmarshal(raw, &frag); err != nil {
		t.Fatal(err)
	}
	digest := ref[len("ghcr.io/actions/actions-runner@"):]
	if frag.Name != "ci-actions-runner" || frag.Kind != "ci" || frag.Owner != "P7-HOST-19" || !regexp.MustCompile(`@`+regexp.QuoteMeta(digest)+`$`).MatchString(frag.Image) ||
		len(frag.Platforms) != 2 {
		t.Fatalf("fragment %+v does not lock %s", frag, ref)
	}
}
