package install

// Purpose: the signed Release source of `nself ci nodes add` (hq NODE D11, P7-NODE-15): it downloads the pinned nself-ci-agent
//          release for a node's platform, verifies it through internal/agent/verify and only then places it.
// Inputs:  a pinned version, a release base URL, a verify.Verifier, the node's lifecycle.Platform.
// Outputs: the path of a verified binary (lifecycle.AgentSource.Fetch) and a cleanup that removes its private directory.
// Constraints: nothing is placed at its final path before verify.Artifact returns nil; every install gets a fresh 0700 directory
//          (os.MkdirTemp, never an existing or shared path, no persistent cache), the download lands there as <name>.part and is
//          moved with one rename (the SBOMs and the signed provenance are fetched first, bounded, and checked by the same
//          verify.Artifact call), so a failed verify leaves no file a later run could mistake for a verified one and no other
//          process can swap the verified file before the installer copies it. https only; plain http is accepted for a loopback host
//          (a local fixture server) and never after a redirect from https. Bodies are size-bounded. The binary is never
//          executed here. With no trusted key the source fails with E665 (reason no_trusted_key) naming --agent-binary.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

// errNotFound marks a release file the server does not have (HTTP 404).
var errNotFound = errors.New("not found")

// BinaryName is the release file prefix: nself-ci-agent-<os>-<arch>.
const BinaryName = "nself-ci-agent"

// DefaultBaseURL is the GitHub release that holds the agent files of one tag: <base>/v<version>/<file>.
const DefaultBaseURL = "https://github.com/nself-org/plugins/releases/download"

// Release downloads, verifies and places the pinned agent. The zero Client is a client with a five-minute timeout.
type Release struct {
	// Version is the pinned release version without a leading v ("1.5.0").
	Version string
	// BaseURL is the directory URL that holds the release files; empty means DefaultBaseURL/v<Version>.
	BaseURL string
	// Verifier decides trust; nil means the compiled-in key set.
	Verifier *verify.Verifier
	Client   *http.Client
}

var _ lifecycle.AgentSource = Release{}

// Fetch implements lifecycle.AgentSource.
func (r Release) Fetch(ctx context.Context, p lifecycle.Platform) (string, func(), error) {
	v := r.Verifier
	if v == nil {
		var err error
		if v, err = verify.Default(); err != nil {
			return "", nil, err
		}
	}
	// Checked first: with no key there is nothing to verify against, so nothing is downloaded.
	if err := v.Ready(); err != nil {
		return "", nil, err
	}
	if !verify.ValidVersion(r.Version) {
		return "", nil, errf(lifecycle.CodeInvalid, "no agent release version is pinned; pass --agent-binary <path>")
	}
	target := verify.Target(p.OS, p.Arch)
	base, err := r.baseURL()
	if err != nil {
		return "", nil, err
	}
	// A fresh private directory per install (mkdir 0700, never an existing path), so no other process can swap the verified file
	// between this return and the installer's copy. The cleanup removes it; the installer calls it after copying.
	dir, err := os.MkdirTemp("", "nself-ci-agent-")
	if err != nil {
		return "", nil, errf(lifecycle.CodeNoRoute, "cannot create a private directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	ok := false
	defer func() {
		if !ok {
			cleanup()
		}
	}()
	name := BinaryName + "-" + target
	manifest, err := r.get(ctx, base+"/"+name+".manifest.json", verify.MaxManifestBytes)
	if err != nil {
		return "", nil, err
	}
	// The SBOMs and the signed provenance (P7-CACHE-10) travel with every release; verify.Artifact refuses the install without them.
	var files verify.Files
	for _, f := range []struct {
		suffix string
		into   *[]byte
		limit  int64
	}{{verify.SuffixProvenance, &files.Provenance, verify.MaxProvenanceBytes}, {verify.SuffixCycloneDX, &files.CycloneDX, verify.MaxSBOMBytes}, {verify.SuffixSPDX, &files.SPDX, verify.MaxSBOMBytes}} {
		// A file the release does not have is left empty: verify.Artifact then refuses with E665 reason provenance. Any other
		// download failure is a transport error (E656).
		if *f.into, err = r.get(ctx, base+"/"+name+f.suffix, f.limit); err != nil && !errors.Is(err, errNotFound) {
			return "", nil, err
		}
	}
	staged := filepath.Join(dir, BinaryName+".part")
	if err = r.download(ctx, base+"/"+name, staged); err != nil {
		return "", nil, err
	}
	if _, err = v.Artifact(staged, manifest, verify.Expect{Version: r.Version, Target: target}, files); err != nil {
		return "", nil, err
	}
	if err = lifecycle.CheckBinary(staged, p); err != nil {
		return "", nil, err
	}
	if err = os.Chmod(staged, 0o700); err != nil {
		return "", nil, errf(lifecycle.CodeNoRoute, "cannot place the agent: %w", err)
	}
	final := filepath.Join(dir, BinaryName)
	if err = os.Rename(staged, final); err != nil {
		return "", nil, errf(lifecycle.CodeNoRoute, "cannot place the agent: %w", err)
	}
	ok = true
	return final, cleanup, nil
}

func (r Release) baseURL() (string, error) {
	base := strings.TrimRight(r.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL + "/v" + r.Version
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || !allowedScheme(u) {
		return "", errf(lifecycle.CodeInvalid, "the agent release URL must be https (or http on a loopback host) without credentials")
	}
	return base, nil
}

// allowedScheme accepts https, and http only when the host is loopback (a fixture server on this machine).
func allowedScheme(u *url.URL) bool {
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (r Release) client() *http.Client {
	c := r.Client
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Minute}
	}
	cp := *c
	cp.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		if !allowedScheme(req.URL) || (via[0].URL.Scheme == "https" && req.URL.Scheme != "https") {
			return fmt.Errorf("redirect to a disallowed URL")
		}
		return nil
	}
	return &cp
}

// open starts a GET and checks the status; the caller closes the body.
func (r Release) open(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, errf(lifecycle.CodeNoRoute, "bad release URL")
	}
	resp, err := r.client().Do(req)
	if err != nil {
		return nil, errf(lifecycle.CodeNoRoute, "cannot download %s: %v", path(u), err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, errf(lifecycle.CodeNoRoute, "download of %s answered 404: %w", path(u), errNotFound)
		}
		return nil, errf(lifecycle.CodeNoRoute, "download of %s answered %d", path(u), resp.StatusCode)
	}
	return resp, nil
}

func path(u string) string {
	if p, err := url.Parse(u); err == nil {
		return p.Path
	}
	return "release file"
}

func (r Release) get(ctx context.Context, u string, limit int64) ([]byte, error) {
	resp, err := r.open(ctx, u)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, errf(lifecycle.CodeNoRoute, "%s is unreadable or larger than %d bytes", path(u), limit)
	}
	return b, nil
}

func (r Release) download(ctx context.Context, u, dst string) error {
	resp, err := r.open(ctx, u)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errf(lifecycle.CodeNoRoute, "cannot create the staging file: %w", err)
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, verify.MaxBinaryBytes+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil || n > verify.MaxBinaryBytes {
		return errf(lifecycle.CodeNoRoute, "%s is unreadable or larger than %d bytes", path(u), int64(verify.MaxBinaryBytes))
	}
	return nil
}

// errf builds an error whose text starts with its code ("E656: ..."), the form lifecycle.CodeOf reads.
func errf(code, format string, a ...any) error { return fmt.Errorf(code+": "+format, a...) }
