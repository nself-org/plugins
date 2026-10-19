package sbom

// Purpose: `nself ci artifacts tools install` for syft: place the pinned
// release at $NSELF_CI_HOME/tools/syft-<version>/syft (Epic CACHE D13).
// Inputs:  the syft Pin (URL and sha256 per os/arch), NSELF_CI_HOME, an HTTP client.
// Outputs: the installed path; E718 tool_checksum when the bytes differ from the
// row's sha256 or the extracted binary differs from its binary_sha256,
// tool_missing when the pin has no source for this os/arch or the
// download fails.
// Constraints: explicit only, never at run time; https only (every redirect
// hop too); the download is size-capped and hashed while it is written to a
// temp file in the target directory, and nothing is extracted or made
// executable before the sha256 matches; only the archive's regular `syft`
// entry is extracted; the final file appears by rename; the installer never
// runs the downloaded binary; a pin without binary_sha256 for the platform is
// refused before any download (D-1024), and the extracted file is hashed
// before the rename. It always re-downloads and replaces, so running
// it again repairs a stale or damaged install; a destination that is a symlink
// or not a regular file is refused, never followed.

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	maxDownload = 300 << 20
	maxBinary   = 512 << 20
)

// InstallOptions configures Install.
type InstallOptions struct {
	Home   string
	Pin    Pin
	OS     string       // default runtime.GOOS
	Arch   string       // default runtime.GOARCH
	Client *http.Client // default: 5 minute timeout
}

// Installed describes the result of Install.
type Installed struct {
	Path     string `json:"path"`
	Version  string `json:"version"`
	SHA256   string `json:"sha256"`
	Archive  string `json:"archive_url"`
	Platform string `json:"platform"`
}

// Install downloads, checks and places the pinned syft.
func Install(ctx context.Context, o InstallOptions) (Installed, error) {
	goos, goarch := o.OS, o.Arch
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	src, ok := o.Pin.Source(goos, goarch)
	if !ok {
		return Installed{}, Failf(ReasonToolMissing, "the %s pin has no source for %s/%s", ToolName, goos, goarch)
	}
	if err := o.Pin.Check(); err != nil {
		return Installed{}, err
	}
	if _, err := o.Pin.wantBinaryDigest(goos, goarch); err != nil {
		return Installed{}, err
	}
	home, err := ResolveHome(o.Home)
	if err != nil {
		return Installed{}, err
	}
	res := Installed{Path: ToolPath(home, o.Pin.Version), Version: o.Pin.Version, SHA256: src.SHA256, Archive: src.URL, Platform: goos + "/" + goarch}
	u, err := url.Parse(src.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return res, Failf(ReasonToolMissing, "the %s source %q is not an https URL", ToolName, src.URL)
	}
	dir := filepath.Dir(res.Path)
	toolsDir := filepath.Dir(dir)
	for _, d := range []string{toolsDir, dir} { // never follow or re-mode a symlinked or foreign directory
		if err := refuseNonRegular(d, true); err != nil {
			return res, err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, Failf(ReasonToolMissing, "cannot create %s: %v", dir, err)
	}
	for _, d := range []string{toolsDir, dir} { // a loose umask must not leave them group-writable
		if err := secureDir(d); err != nil {
			return res, err
		}
	}
	if err := refuseNonRegular(res.Path, false); err != nil {
		return res, err
	}
	sweepStale(dir)
	tmp, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return res, Failf(ReasonToolMissing, "cannot write in %s: %v", dir, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	sum, err := download(ctx, o.Client, src.URL, tmp)
	_ = tmp.Close()
	if err != nil {
		return res, err
	}
	if sum != src.SHA256 {
		return res, Failf(ReasonToolChecksum, "the %s download from %s has sha256 %s, the pin says %s; refusing to install it", ToolName, src.URL, sum, src.SHA256)
	}
	if err := extractBinary(tmp.Name(), res.Path, src.BinarySHA256); err != nil {
		return res, err
	}
	return res, nil
}

// refuseNonRegular refuses an existing path that is a symlink or the wrong type.
func refuseNonRegular(p string, wantDir bool) error {
	st, err := os.Lstat(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return Failf(ReasonToolMissing, "cannot inspect %s: %v", p, err)
	}
	if st.IsDir() != wantDir || st.Mode()&os.ModeSymlink != 0 || (!wantDir && !st.Mode().IsRegular()) {
		return Failf(ReasonToolMissing, "%s exists and is not a plain %s; remove it and run `%s` again", p, map[bool]string{true: "directory", false: "file"}[wantDir], InstallCommand)
	}
	return nil
}

// secureDir sets mode 0755 on a plain directory the current user owns.
func secureDir(d string) error {
	st, err := os.Lstat(d)
	if err != nil {
		return Failf(ReasonToolMissing, "cannot inspect %s: %v", d, err)
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || !ownedByUs(st) {
		return Failf(ReasonToolMissing, "%s is not a plain directory owned by the current user; remove it and run `%s` again", d, InstallCommand)
	}
	if err := os.Chmod(d, 0o755); err != nil {
		return Failf(ReasonToolMissing, "cannot secure %s: %v", d, err)
	}
	return nil
}

// httpsOnly returns a copy of c that refuses a redirect to anything but https.
func httpsOnly(c *http.Client) *http.Client {
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Minute}
	}
	cc := *c
	prev := cc.CheckRedirect
	cc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("refusing a redirect to %s (not https)", req.URL.Redacted())
		}
		if prev != nil {
			return prev(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &cc
}

func download(ctx context.Context, c *http.Client, rawURL string, w io.Writer) (string, error) {
	c = httpsOnly(c)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", Failf(ReasonToolMissing, "bad download URL: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", Failf(ReasonToolMissing, "download failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", Failf(ReasonToolMissing, "download failed: HTTP %d", resp.StatusCode)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return "", Failf(ReasonToolMissing, "download failed: %v", err)
	}
	if n > maxDownload {
		return "", Failf(ReasonToolChecksum, "the download is larger than %d bytes", maxDownload)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractBinary writes the archive's regular `syft` entry to dst (mode 0755)
// when its sha256 is wantSum.
func extractBinary(archive, dst, wantSum string) error {
	f, err := os.Open(archive)
	if err != nil {
		return Failf(ReasonToolMissing, "cannot read the download: %v", err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return Failf(ReasonToolChecksum, "the download is not a gzip archive: %v", err)
	}
	tr := tar.NewReader(io.LimitReader(gz, 2*maxBinary)) // a verified archive may still hold a decompression bomb
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return Failf(ReasonToolChecksum, "the archive has no regular %q file", ToolName)
		}
		if err != nil {
			return Failf(ReasonToolChecksum, "the archive is damaged: %v", err)
		}
		if h.Typeflag != tar.TypeReg || path.Clean(h.Name) != ToolName {
			continue
		}
		if h.Size <= 0 || h.Size > maxBinary {
			return Failf(ReasonToolChecksum, "the archive's %s file has size %d", ToolName, h.Size)
		}
		return place(tr, h.Size, dst, wantSum)
	}
}

func place(r io.Reader, size int64, dst, wantSum string) error {
	out, err := os.CreateTemp(filepath.Dir(dst), ".syft-*")
	if err != nil {
		return Failf(ReasonToolMissing, "cannot write %s: %v", dst, err)
	}
	name := out.Name()
	fail := func(err error) error {
		_ = out.Close()
		_ = os.Remove(name)
		return Failf(ReasonToolMissing, "cannot write %s: %v", dst, err)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(r, size))
	if err != nil {
		return fail(err)
	}
	if n != size {
		return fail(fmt.Errorf("short file: %d of %d bytes", n, size))
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantSum {
		_ = out.Close()
		_ = os.Remove(name)
		return Failf(ReasonToolChecksum, "the extracted %s binary has sha256 %s, the pin says %s; refusing to install it", ToolName, got, wantSum)
	}
	if err := out.Chmod(0o755); err != nil {
		return fail(err)
	}
	if err := out.Close(); err != nil {
		return fail(err)
	}
	if err := os.Rename(name, dst); err != nil {
		_ = os.Remove(name)
		return Failf(ReasonToolMissing, "cannot place %s: %v", dst, err)
	}
	return nil
}

// sweepStale removes temp files an interrupted install left behind (older than an hour).
func sweepStale(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() || time.Since(info.ModTime()) < time.Hour {
			continue
		}
		if n := e.Name(); strings.HasPrefix(n, ".download-") || strings.HasPrefix(n, ".syft-") {
			_ = os.Remove(filepath.Join(dir, n))
		}
	}
}
