package sbom

// Verify the pinned executable before it is used.
import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// wantBinaryDigest is the pin's sha256 of the extracted executable for an os/arch.
func (p Pin) wantBinaryDigest(goos, goarch string) (string, error) {
	src, ok := p.Source(goos, goarch)
	if !ok || src.BinarySHA256 == "" {
		return "", Failf(ReasonToolChecksum, "the %s pin has no binary_sha256 for %s/%s, so an installed binary cannot be authenticated", ToolName, goos, goarch)
	}
	return src.BinarySHA256, nil
}

// verifyBinary hashes the file at p and compares it with the pin's binary_sha256.
// It hashes the descriptor it opened and refuses a path that changed under it.
func verifyBinary(p string, pin Pin, goos, goarch string) error {
	want, err := pin.wantBinaryDigest(goos, goarch)
	if err != nil {
		return err
	}
	pre, err := os.Lstat(p)
	if err != nil {
		return Failf(ReasonToolMissing, "cannot inspect %s: %v", p, err)
	}
	f, err := os.Open(p)
	if err != nil {
		return Failf(ReasonToolMissing, "cannot read %s: %v", p, err)
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || !os.SameFile(pre, st) {
		return Failf(ReasonToolChecksum, "%s changed while it was being checked; run `%s`", p, InstallCommand)
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, maxBinary+1))
	if err != nil || n > maxBinary {
		return Failf(ReasonToolChecksum, "cannot hash %s (%d bytes read, error %v); run `%s`", p, n, err, InstallCommand)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return Failf(ReasonToolChecksum, "%s has sha256 %s, the pin says %s; it is not the pinned %s %s; run `%s` to replace it", p, got, want, ToolName, pin.Version, InstallCommand)
	}
	return nil
}
