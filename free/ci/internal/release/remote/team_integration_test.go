//go:build integration

package remote_test

// Purpose: the acceptance of P7-CACHE-17 against the simulated TEAM coordinator (rig_integration_test.go):
// a release-role operator signs a release run and the result verifies; a member is refused on both
// routes; a blob altered on the coordinator is caught by the local re-hash and nothing is signed;
// the coordinator never calls its secret resolver and never sees a key; every refusal of a run or
// blob the caller may not see is the same E713 as "not found".

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	"github.com/nself-org/plugins/free/ci/internal/release/remote"
	"github.com/nself-org/plugins/free/ci/internal/release/verify"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/signing"
)

func sumHex(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// containsKey reports whether b holds the release key value, raw or base64.
func (tm *team) containsKey(b []byte) bool {
	v := tm.keyValue
	for _, needle := range [][]byte{v, []byte(base64.StdEncoding.EncodeToString(v)), []byte(base64.RawURLEncoding.EncodeToString(v)), []byte(base64.URLEncoding.EncodeToString(v))} {
		if len(needle) > 8 && bytes.Contains(b, needle) {
			return true
		}
	}
	return false
}

// keyOnDisk walks dir and reports the first file that holds the key value.
func (tm *team) keyOnDisk(dir string) (string, int) {
	var hit string
	files := 0
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		files++
		if hit == "" && tm.containsKey(b) {
			hit = p
		}
		return nil
	})
	return hit, files
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func asRemote(t *testing.T, err error) *remote.Error {
	t.Helper()
	var re *remote.Error
	if err == nil || !errors.As(err, &re) {
		t.Fatalf("want a *remote.Error, got %v", err)
	}
	return re
}

func (tm *team) verifyDir(dir string) verify.Result {
	info, err := signing.Public(context.Background(), tm.opRes, signing.KeyAttempt)
	if err != nil {
		tm.t.Fatal(err)
	}
	keys := filepath.Join(tm.t.TempDir(), "ci-release.keys")
	if err = os.WriteFile(keys, []byte(info.TrustLine()+"\n"), 0o600); err != nil {
		tm.t.Fatal(err)
	}
	return verify.Release(context.Background(), dir, verify.Options{Root: tm.repo, Capture: report.Capture, OperatorKeys: keys})
}

func readTree(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	if err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		b, err := os.ReadFile(p)
		out[filepath.ToSlash(rel)] = b
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestTeamSignRelease: an operator with the release role signs a TEAM release on their own machine;
// the directory verifies (library and the shipped `ci artifacts verify`), equals what the coordinator
// itself would have signed with the same key, and the coordinator held no key and made no call to its resolver.
func TestTeamSignRelease(t *testing.T) {
	tm := newTeam(t)
	s := newSim(t, tm, nil)
	out := filepath.Join(t.TempDir(), "out")
	p := s.params(s.client(roles.Release, project), out)
	rel, err := remote.Sign(context.Background(), p)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if rel.State != publish.Published || rel.RunID != tm.runID || rel.KeyID == "" {
		t.Fatalf("release %+v", rel)
	}
	if a, _ := filepath.EvalSymlinks(rel.Dir); a != mustEval(t, out) {
		t.Fatalf("release written to %q, want %q", rel.Dir, out)
	}
	files := readTree(t, out)
	for _, want := range []string{"release-manifest.dsse.json", "evidence.json", "provenance.intoto.dsse.json", "SHA256SUMS", "sbom/app.cdx.json", "sbom/app.spdx.json", "artifacts/app"} {
		if _, ok := files[want]; !ok {
			t.Fatalf("release lacks %s", want)
		}
	}
	if string(files["artifacts/app"]) != "app-bytes" {
		t.Fatalf("artifact bytes %q", files["artifacts/app"])
	}
	if v := tm.verifyDir(out); !v.Verified || v.Exit != 0 {
		t.Fatalf("artifacts verify: %+v", v)
	}
	tm.cliVerify(out)

	// the same key signs the same bytes: the TEAM release equals the one the coordinator would have made
	d := tm.coordDeps(tm.st)
	d.Team, d.Home = false, t.TempDir()
	d.Signer = remote.NewLocalSigner(tm.st, d.Blobs, tm.opRes, tm.repo)
	local, err := publish.Run(context.Background(), d, tm.runID)
	if err != nil || local.ManifestSHA256 != rel.ManifestSHA256 {
		t.Fatalf("local %+v %v vs remote %s", local, err, rel.ManifestSHA256)
	}
	if want := readTree(t, local.Dir); len(want) != len(files) {
		t.Fatalf("file sets differ: %d vs %d", len(want), len(files))
	} else {
		for name, b := range want {
			if !bytes.Equal(files[name], b) {
				t.Fatalf("%s differs between the TEAM release and the local one", name)
			}
		}
	}

	// the coordinator: GET requests only, no body, no key in any answer, no resolver call, no release written
	if n := tm.coordRes.calls.Load(); n != 0 {
		t.Fatalf("the coordinator's secret resolver was called %d times", n)
	}
	if tm.opRes.calls.Load() == 0 {
		t.Fatal("the operator's key was never resolved")
	}
	for i, r := range s.rec.requests {
		if !strings.HasPrefix(r, "GET /v1/") || len(s.rec.bodies[i]) != 0 {
			t.Fatalf("the coordinator received %q with %d body bytes", r, len(s.rec.bodies[i]))
		}
	}
	if tm.containsKey(s.rec.answered.Bytes()) {
		t.Fatal("the key value crossed the wire")
	}
	if hit, n := tm.keyOnDisk(tm.home); hit != "" || n == 0 {
		t.Fatalf("coordinator home: key found in %q (files read %d)", hit, n)
	}
	if exists(publish.Dir(tm.home, tm.runID)) && !strings.HasPrefix(local.Dir, tm.home) {
		t.Fatal("the TEAM coordinator wrote a release directory")
	}
	left, _ := filepath.Glob(filepath.Join(p.OperatorHome, ".team-sign-*"))
	if len(left) != 0 {
		t.Fatalf("private work directories left behind: %v", left)
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	if exists(p) {
		r, err := filepath.EvalSymlinks(p)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	d, err := filepath.EvalSymlinks(filepath.Dir(p))
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(d, filepath.Base(p))
}

// cliVerify runs the shipped binary's `ci artifacts verify <dir>` on the signed directory.
func (tm *team) cliVerify(dir string) {
	t := tm.t
	bin := filepath.Join(t.TempDir(), "nself-ci")
	build := exec.Command("go", "build", "-mod=vendor", "-o", bin, "../../../cmd")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	info, err := signing.Public(context.Background(), tm.opRes, signing.KeyAttempt)
	if err != nil {
		t.Fatal(err)
	}
	keys := filepath.Join(t.TempDir(), "ci-release.keys")
	if err = os.WriteFile(keys, []byte(info.TrustLine()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "artifacts", "verify", dir, "--keys", keys, "--repo", tm.repo, "--json")
	cmd.Dir = tm.repo
	cmd.Env = append(os.Environ(), "NSELF_V15=1", "NSELF_CI_HOME="+t.TempDir(), "NSELF_CI_REPO="+tm.repo)
	out, err := cmd.CombinedOutput()
	var env struct {
		Data struct {
			Verified bool `json:"verified"`
		} `json:"data"`
	}
	if err != nil || json.Unmarshal(out, &env) != nil || !env.Data.Verified {
		t.Fatalf("ci artifacts verify: %v\n%s", err, out)
	}
}

// TestTeamSignMemberRefused: a member is refused on both routes with E687 (role), an unauthenticated
// request never reaches a handler, and nothing is written or signed.
func TestTeamSignMemberRefused(t *testing.T) {
	tm := newTeam(t)
	s := newSim(t, tm, nil)
	member := s.client(roles.Member, project)
	// an unknown run answers a member exactly as a known one does: the role is checked before any run is looked up
	for _, path := range []string{remote.InputsPath(tm.runID), remote.BlobPath(tm.runID, sumHex([]byte("app-bytes"))), remote.InputsPath("run-nope"), remote.BlobPath("run-nope", strings.Repeat("0", 64))} {
		resp, err := member.GetRaw(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		var b clientapi.Body
		_ = json.NewDecoder(resp.Body).Decode(&b)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden || b.Code != roles.CodeNotAuthorized || b.Reason != roles.ReasonRole {
			t.Fatalf("%s: status %d body %+v, want 403 E687 role", path, resp.StatusCode, b)
		}
	}
	out := filepath.Join(t.TempDir(), "out")
	_, err := remote.Sign(context.Background(), s.params(member, out))
	if re := asRemote(t, err); re.ErrCode != roles.CodeNotAuthorized || re.Reason != roles.ReasonRole {
		t.Fatalf("member sign: %v", err)
	}
	if exists(out) || tm.opRes.calls.Load() != 0 || tm.coordRes.calls.Load() != 0 {
		t.Fatalf("a refused member left output or resolved a key (out %v, operator %d, coordinator %d)", exists(out), tm.opRes.calls.Load(), tm.coordRes.calls.Load())
	}

	// no credential: the listener refuses before any handler runs
	pool := x509.NewCertPool()
	pool.AddCert(s.ca.Root())
	hc := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13}}}
	resp, err := hc.Get(s.url + remote.InputsPath(tm.runID))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(string(raw), "no_credential") {
		t.Fatalf("unauthenticated: %d %s", resp.StatusCode, raw)
	}
}

// tamper serves one blob with a flipped byte (the same size): the coordinator's bytes changed after the run.
type tamper struct {
	inner  *cas.Store
	digest string
	cut    bool
}

func (t tamper) Stat(r cas.Realm, d string) (os.FileInfo, error) { return t.inner.Stat(r, d) }
func (t tamper) Open(r cas.Realm, d string) (io.ReadCloser, error) {
	rc, err := t.inner.Open(r, d)
	if err != nil || d != t.digest {
		return rc, err
	}
	defer func() { _ = rc.Close() }()
	b, _ := io.ReadAll(rc)
	if t.cut {
		b = b[:len(b)/2]
	} else {
		b[0] ^= 0xff
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

// TestTeamSignTampered: a blob altered on the coordinator after the run fails the local re-hash with
// E719 checksum, and nothing is signed: no output, the operator's key is never resolved.
func TestTeamSignTampered(t *testing.T) {
	for name, cut := range map[string]bool{"flipped byte": false, "truncated": true} {
		t.Run(name, func(t *testing.T) {
			tm := newTeam(t)
			s := newSim(t, tm, tamper{inner: tm.blobs, digest: sumHex([]byte("app-bytes")), cut: cut})
			out := filepath.Join(t.TempDir(), "out")
			_, err := remote.Sign(context.Background(), s.params(s.client(roles.Release, project), out))
			re := asRemote(t, err)
			if !cut && (re.ErrCode != verify.Code || re.Reason != verify.ReasonChecksum) {
				t.Fatalf("want E719 checksum, got %v", err)
			}
			if exists(out) || tm.opRes.calls.Load() != 0 {
				t.Fatalf("a tampered blob was signed or left output (out %v, key resolutions %d)", exists(out), tm.opRes.calls.Load())
			}
			if row, _, _ := tm.st.ReleaseFor(context.Background(), tm.runID); row.State == "published" {
				t.Fatalf("the coordinator's ledger says %+v", row)
			}
		})
	}
}

// TestTeamSignHidesWhatTheCallerMaySee: another project's release holder, an unknown run, a digest that
// is not in the run's release inputs, a cache-realm blob and a malformed digest all get the same E713
// body as "not found".
func TestTeamSignHidesWhatTheCallerMaySee(t *testing.T) {
	tm := newTeam(t)
	s := newSim(t, tm, nil)
	realm := cas.CacheRealm("ns-1")
	cacheDigest, _, err := tm.blobs.Put(context.Background(), realm, strings.NewReader("cache bytes"))
	if err != nil {
		t.Fatal(err)
	}
	app := sumHex([]byte("app-bytes"))
	own, other := s.client(roles.Release, project), s.client(roles.Release, "other/proj")
	get := func(c remote.Fetcher, path string) (int, []byte) {
		resp, err := c.(interface {
			GetRaw(context.Context, string) (*http.Response, error)
		}).GetRaw(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b
	}
	_, want := get(own, remote.BlobPath(tm.runID, strings.Repeat("0", 64)))
	if !strings.Contains(string(want), `"E713"`) {
		t.Fatalf("not-found body %s", want)
	}
	// a blob of the run's own artifact realm that is not a release input (a log, say) is not served either
	_, inputsRaw := get(own, remote.InputsPath(tm.runID))
	var in remote.Inputs
	if err = json.Unmarshal(inputsRaw, &in); err != nil || in.Pipeline.Project == "" {
		t.Fatalf("inputs: %v %s", err, inputsRaw)
	}
	logDigest, _, err := tm.blobs.Put(context.Background(), cas.ArtifactRealm(in.Pipeline.Project), strings.NewReader("a log line"))
	if err != nil {
		t.Fatal(err)
	}
	type probe struct {
		who  remote.Fetcher
		path string
	}
	for name, c := range map[string]probe{
		"other project's inputs":  {other, remote.InputsPath(tm.runID)},
		"other project's blob":    {other, remote.BlobPath(tm.runID, app)},
		"unknown run inputs":      {own, remote.InputsPath("run-nope")},
		"unknown run blob":        {own, remote.BlobPath("run-nope", app)},
		"cache-realm digest":      {own, remote.BlobPath(tm.runID, cacheDigest)},
		"unlisted artifact-realm": {own, remote.BlobPath(tm.runID, logDigest)},
		"malformed digest":        {own, remote.BlobPath(tm.runID, "ZZ")},
		"upper-case digest":       {own, remote.BlobPath(tm.runID, strings.ToUpper(app))},
		"digest of the evidence":  {own, remote.BlobPath(tm.runID, sumHex([]byte("evidence")))},
	} {
		if status, body := get(c.who, c.path); status != http.StatusNotFound || !bytes.Equal(body, want) {
			t.Errorf("%s: status %d body %s, want 404 %s", name, status, body, want)
		}
	}
	// a path the router itself refuses never reaches the bytes
	for _, path := range []string{"/v1/runs/..%2f..%2fx/release-inputs", remote.BlobPath(tm.runID, app+"/x")} {
		if status, body := get(own, path); status == http.StatusOK || bytes.Contains(body, []byte("app-bytes")) {
			t.Errorf("%s: status %d body %s", path, status, body)
		}
	}
	// and the same digest, asked for the right run by the right project, is served
	if status, body := get(own, remote.BlobPath(tm.runID, app)); status != http.StatusOK || string(body) != "app-bytes" {
		t.Fatalf("own blob: %d %q", status, body)
	}
}

// TestTeamSignWrongRepository: a run of another repository is refused before the release key is resolved.
func TestTeamSignWrongRepository(t *testing.T) {
	tm := newTeam(t)
	s := newSim(t, tm, nil)
	elsewhere, _ := newRepo(t, "a different repository\n")
	out := filepath.Join(t.TempDir(), "out")
	p := s.params(s.client(roles.Release, project), out)
	p.Root = elsewhere
	_, err := remote.Sign(context.Background(), p)
	if re := asRemote(t, err); re.ErrCode != signing.CodeSigning || !strings.Contains(re.Msg, "belongs to repository") {
		t.Fatalf("want a repository refusal, got %v", err)
	}
	if exists(out) || tm.opRes.calls.Load() != 0 {
		t.Fatalf("a foreign repository reached the key (out %v, resolutions %d)", exists(out), tm.opRes.calls.Load())
	}
}
