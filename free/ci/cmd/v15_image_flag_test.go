package main

import (
	"os"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

const testImageRef = "ghcr.io/nself-org/ci-toolchain@sha256:" +
	"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestImageFlagRegistered(t *testing.T) {
	if bareFlags.Lookup("image") == nil {
		t.Fatal("bare flag set has no --image (the E645 text tells users to pass it)")
	}
}

// --image reaches the toolchain resolver through its one input, NSELF_CI_IMAGE.
func TestImageFlagSetsResolverInput(t *testing.T) {
	t.Setenv(toolchain.EnvImage, "")
	o, err := parseBare([]string{"--container", "--image", testImageRef})
	if err != nil {
		t.Fatal(err)
	}
	if !o.Container {
		t.Error("--container was lost")
	}
	if got := os.Getenv(toolchain.EnvImage); got != testImageRef {
		t.Errorf("%s = %q, want the --image value", toolchain.EnvImage, got)
	}
}

// The flag wins over an image the environment already names.
func TestImageFlagBeatsEnvironment(t *testing.T) {
	t.Setenv(toolchain.EnvImage, "example.invalid/old@sha256:"+strings.Repeat("a", 64))
	if _, err := parseBare([]string{"--container", "--image", testImageRef}); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(toolchain.EnvImage); got != testImageRef {
		t.Errorf("%s = %q, want the --image value", toolchain.EnvImage, got)
	}
}

// Without the flag the environment value stays, and a flag from an earlier
// parse in the same process does not leak into the next one.
func TestImageFlagAbsentKeepsEnvironment(t *testing.T) {
	t.Setenv(toolchain.EnvImage, "")
	if _, err := parseBare([]string{"--image", testImageRef}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(toolchain.EnvImage, "from-env")
	if _, err := parseBare([]string{"--container"}); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(toolchain.EnvImage); got != "from-env" {
		t.Errorf("%s = %q, want the untouched environment value", toolchain.EnvImage, got)
	}
}

func TestImageFlagRefusesEmpty(t *testing.T) {
	if _, err := parseBare([]string{"--image", ""}); err == nil {
		t.Error("--image with no value was accepted")
	}
}
