package toolchain

// Purpose: the toolchain-pins drill prints one valid ci.evidence document with
// one check per versions.yaml row; a missing artifact fails, a newer release is
// advisory only. The script runs against a local mirror (DRILL_MIRROR).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

const drillScript = "../../../../scripts/drill/toolchain-pins/run"

func fixtureTable(goneURL string) string {
	h := strings.Repeat("cd", 32)
	return `version: 1
rows:
  - {name: alpine, kind: base-image, version: "3.22", purpose: p, owner: o, image: docker.io/library/alpine, digest: "sha256:` + h + `", upstream: {type: oci, ref: x}}
  - name: demo
    kind: tool
    version: "1.2.3"
    purpose: p
    owner: o
    probe: [demo, --version]
    upstream: {type: github, ref: acme/demo}
    sources:
      - {os: linux, arch: amd64, url: "https://example.org/acme/demo-1.2.3.tgz", sha256: "` + h + `"}
  - name: gone
    kind: tool
    version: "2.0.0"
    purpose: p
    owner: o
    probe: [gone, --version]
    upstream: {type: github, ref: acme/gone}
    sources:
      - {os: linux, arch: amd64, url: "` + goneURL + `", sha256: "` + h + `"}
  - {name: toolchain-image, kind: image, purpose: p, owner: o, image: docker.io/nself/ci-toolchain, status: pending, upstream: {type: none, ref: ""}}
`
}

func mirror(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api.github.com/repos/acme/demo/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.3.0"}`))
	})
	mux.HandleFunc("/api.github.com/repos/acme/gone/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v2.0.0"}`))
	})
	mux.HandleFunc("/example.org/acme/demo-1.2.3.tgz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("x")) })
	mux.HandleFunc("/auth.docker.io/token", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"token":"t"}`)) })
	mux.HandleFunc("/registry-1.docker.io/v2/library/alpine/manifests/", func(w http.ResponseWriter, _ *http.Request) {})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func runDrill(t *testing.T, table string, srv *httptest.Server) (doc map[string]any, exit int, raw []byte) {
	t.Helper()
	for _, tool := range []string{"curl", "jq", "yq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("the drill test needs %s on PATH: %v", tool, err)
		}
	}
	path := filepath.Join(t.TempDir(), "versions.yaml")
	if err := os.WriteFile(path, []byte(table), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", drillScript)
	cmd.Env = append(os.Environ(), "TOOLCHAIN_VERSIONS="+path, "DRILL_MIRROR="+srv.URL, "TMPDIR="+t.TempDir())
	out, err := cmd.Output()
	if ee, ok := err.(*exec.ExitError); ok {
		exit = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	if e := json.Unmarshal(out, &doc); e != nil {
		t.Fatalf("not one JSON document: %v\n%s", e, out)
	}
	return doc, exit, out
}

func validEvidence(t *testing.T, raw []byte) {
	t.Helper()
	schemas, err := model.Schemas()
	if err != nil {
		t.Fatal(err)
	}
	if err := model.ValidateJSON(schemas["ci-evidence.v1.schema.json"], raw); err != nil {
		t.Fatalf("not a valid ci.evidence/v1 document: %v\n%s", err, raw)
	}
}

func resultsByTool(doc map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, c := range doc["checks"].([]any) {
		m := c.(map[string]any)
		out[m["tool"].(string)] = m
	}
	return out
}

func TestDrillAdvisoryNewerReleaseAndPendingImage(t *testing.T) {
	srv := mirror(t)
	table := fixtureTable("https://example.org/acme/demo-1.2.3.tgz")
	doc, exit, raw := runDrill(t, table, srv)
	validEvidence(t, raw)
	if exit != 0 || doc["result"] != "pass" {
		t.Fatalf("exit %d result %v\n%s", exit, doc["result"], raw)
	}
	checks := resultsByTool(doc)
	if len(checks) != 4 {
		t.Fatalf("want one check per row (4), got %d: %v", len(checks), checks)
	}
	if c := checks["demo"]; c["result"] != "pass" || c["findings"].(float64) != 1 || !strings.Contains(c["excerpt"].(string), "1.3.0") {
		t.Errorf("a newer release must be an advisory finding, got %v", c)
	}
	if c := checks["gone"]; c["result"] != "pass" || c["findings"].(float64) != 0 {
		t.Errorf("current pin must pass clean, got %v", c)
	}
	if c := checks["toolchain-image"]; c["result"] != "skip" || c["required"] != false {
		t.Errorf("pending image must be a non-required skip, got %v", c)
	}
	if c := checks["alpine"]; c["result"] != "pass" {
		t.Errorf("resolvable base digest must pass, got %v", c)
	}
}

func TestDrillFailsWhenAnArtifactIsGone(t *testing.T) {
	srv := mirror(t)
	doc, exit, raw := runDrill(t, fixtureTable("https://example.org/acme/removed-2.0.0.tgz"), srv)
	validEvidence(t, raw)
	if exit != 1 || doc["result"] != "fail" {
		t.Fatalf("exit %d result %v\n%s", exit, doc["result"], raw)
	}
	if c := resultsByTool(doc)["gone"]; c["result"] != "fail" || !strings.Contains(c["excerpt"].(string), "no longer downloadable") {
		t.Errorf("gone artifact check = %v", c)
	}
}
