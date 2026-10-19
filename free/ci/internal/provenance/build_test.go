package provenance_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/provenance"
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
)

func TestBuildValidatesAgainstVendoredSchemas(t *testing.T) {
	checkSchemas(t, mustBuild(t, releaseFacts()))
	b, err := provenance.BuildAgent(agentFacts())
	if err != nil {
		t.Fatal(err)
	}
	checkSchemas(t, b)
}

func TestSchemasRefuseABrokenStatement(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(mustBuild(t, releaseFacts()), &doc); err != nil {
		t.Fatal(err)
	}
	if err := schemaError(t, "testdata/in-toto-statement-v1.schema.json", doc); err != nil {
		t.Fatal(err)
	}
	doc["subject"] = []any{}
	if schemaError(t, "testdata/in-toto-statement-v1.schema.json", doc) == nil {
		t.Fatal("an empty subject passed the Statement schema")
	}
	delete(doc, "_type")
	if schemaError(t, "testdata/in-toto-statement-v1.schema.json", doc) == nil {
		t.Fatal("a missing _type passed the Statement schema")
	}
	if schemaError(t, "testdata/slsa-provenance-v1.schema.json", map[string]any{"buildDefinition": map[string]any{}}) == nil {
		t.Fatal("an incomplete predicate passed the provenance schema")
	}
}

func TestBuildNamesRevisionLockfilesAndToolchainImage(t *testing.T) {
	f := releaseFacts()
	st, err := provenance.Parse(mustBuild(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if st.GitCommit() != f.Revision {
		t.Fatalf("git commit %q", st.GitCommit())
	}
	if d, ok := st.Dependency("go.sum"); !ok || d != hex64("1") {
		t.Fatalf("go.sum %q %v", d, ok)
	}
	if d, ok := st.Dependency("pnpm-lock.yaml"); !ok || d != hex64("2") {
		t.Fatalf("pnpm-lock %q %v", d, ok)
	}
	deps := st.Predicate.BuildDefinition.ResolvedDependencies
	last := deps[len(deps)-1]
	if last.URI != "ghcr.io/example/toolchain:1" || last.Digest["sha256"] != hex64("3") || len(deps) != 4 {
		t.Fatalf("toolchain image dependency: %+v", deps)
	}
	if deps[0].URI != "git+"+f.RepoID+"@"+f.Revision {
		t.Fatalf("source dependency %q", deps[0].URI)
	}
	if ev, ok := st.Byproduct("evidence.json"); !ok || ev != hex64("6") {
		t.Fatalf("evidence byproduct %q", ev)
	}
	for _, name := range []string{"sbom/image/nself.cdx.json", "sbom/image/nself.spdx.json", "sbom/nself-linux-amd64.cdx.json", "sbom/nself-linux-amd64.spdx.json"} {
		if _, ok := st.Byproduct(name); !ok {
			t.Fatalf("byproduct %s missing", name)
		}
	}
	ext := st.Predicate.BuildDefinition.ExternalParameters
	if ext["repository"] != f.RepoID || ext["revision"] != f.Revision || ext["pipeline"] != "release" || ext["job"] != "publish" || ext["config_digest"] != hex64("c") {
		t.Fatalf("external parameters %v", ext)
	}
	if st.Predicate.BuildDefinition.InternalParameters["isolation"] != "container" || st.Predicate.BuildDefinition.InternalParameters["nself_ci_version"] != "1.5.0" {
		t.Fatalf("internal parameters %v", st.Predicate.BuildDefinition.InternalParameters)
	}
	md := st.Predicate.RunDetails.Metadata
	if md.InvocationID != "run-1/run-1.publish-1a2b3c4d.a1" || md.StartedOn != "2026-10-19T11:00:05Z" || md.FinishedOn != "2026-10-19T12:00:00Z" {
		t.Fatalf("metadata %+v", md)
	}
	if st.Predicate.BuildDefinition.BuildType != provenance.BuildTypeRelease {
		t.Fatal("build type")
	}
}

func TestBuildWithoutAnImageHasNoImageDependency(t *testing.T) {
	f := releaseFacts()
	f.Image = nil
	st, err := provenance.Parse(mustBuild(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(st.Predicate.BuildDefinition.ResolvedDependencies); n != 3 {
		t.Fatalf("%d dependencies, want source and two lockfiles", n)
	}
}

func TestBuildIsDeterministicWhateverTheInputOrder(t *testing.T) {
	a := mustBuild(t, releaseFacts())
	f := releaseFacts()
	f.Subjects[0], f.Subjects[1] = f.Subjects[1], f.Subjects[0]
	f.Lockfiles[0], f.Lockfiles[1] = f.Lockfiles[1], f.Lockfiles[0]
	f.SBOMs[0], f.SBOMs[3] = f.SBOMs[3], f.SBOMs[0]
	if b := mustBuild(t, f); !bytes.Equal(a, b) {
		t.Fatalf("order changed the bytes:\n%s\n%s", a, b)
	}
	if bytes.ContainsAny(a, "\n\t") || bytes.Contains(a, []byte(": ")) {
		t.Fatal("insignificant whitespace in the statement")
	}
	var back map[string]any
	if json.Unmarshal(a, &back) != nil {
		t.Fatal("not JSON")
	}
	re, _ := json.Marshal(back) // encoding/json writes sorted keys without whitespace
	if string(re) != string(a) {
		t.Fatalf("keys are not sorted:\n%s\n%s", a, re)
	}
}

func TestBuildTruncatesTimesToWholeSecondsInUTC(t *testing.T) {
	f := releaseFacts()
	zone := time.FixedZone("x", 5*3600)
	f.StartedOn, f.FinishedOn = time.Date(2026, 10, 19, 16, 0, 5, 999, zone), time.Date(2026, 10, 19, 17, 0, 0, 0, zone)
	st, err := provenance.Parse(mustBuild(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if md := st.Predicate.RunDetails.Metadata; md.StartedOn != "2026-10-19T11:00:05Z" || md.FinishedOn != "2026-10-19T12:00:00Z" {
		t.Fatalf("%+v", md)
	}
}

func TestBuilderIDIsPseudonymousAndStablePerRunner(t *testing.T) {
	f := releaseFacts()
	first := mustBuild(t, f)
	f.RunID, f.AttemptID = "run-2", "run-2.publish-1a2b3c4d.a1"
	second := mustBuild(t, f)
	id := func(b []byte) string {
		st, err := provenance.Parse(b)
		if err != nil {
			t.Fatal(err)
		}
		return st.Predicate.RunDetails.Builder.ID
	}
	if id(first) != id(second) {
		t.Fatalf("builder id changed between runs: %s %s", id(first), id(second))
	}
	if !regexp.MustCompile(`^https://nself\.org/ci/builder/v1#coordinator-[0-9a-f]{12}/runner-[0-9a-f]{12}$`).MatchString(id(first)) {
		t.Fatalf("builder id %q", id(first))
	}
	f.Runner = "runner-8"
	if id(mustBuild(t, f)) == id(first) {
		t.Fatal("two runners share one builder id")
	}
	for _, leak := range []string{"camarata", "runner-7", "ms5"} {
		if strings.Contains(string(first), leak) {
			t.Fatalf("statement leaks %q", leak)
		}
	}
}

func TestPrivateRepositoryKeepsTheRawBuilderIDs(t *testing.T) {
	f := releaseFacts()
	f.Visibility = attest.Private
	f.Pipeline = "/Users/admin/build 192.168.1.20"
	st, err := provenance.Parse(mustBuild(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Predicate.RunDetails.Builder.ID; got != "https://nself.org/ci/builder/v1#ms5.camarata.lan/runner-7" {
		t.Fatalf("private builder id %q", got)
	}
}

func TestPublicRepositoryRefusesPrivateFacts(t *testing.T) {
	for name, mutate := range map[string]func(*provenance.ReleaseFacts){
		"rfc1918 in the pipeline name": func(f *provenance.ReleaseFacts) { f.Pipeline = "build-10.1.2.3" },
		"home path in a lockfile":      func(f *provenance.ReleaseFacts) { f.Lockfiles[0].Name = "/Users/admin/work/go.sum" },
		"private host in the image":    func(f *provenance.ReleaseFacts) { f.Image.Ref = "registry.corp.local/tool:1" },
		"literal user name":            func(f *provenance.ReleaseFacts) { f.Job = "publish-by-ali"; f.Usernames = []string{"ali"} },
		"literal host name":            func(f *provenance.ReleaseFacts) { f.Isolation = "boxy"; f.Hostnames = []string{"boxy"} },
		"home path in a subject":       func(f *provenance.ReleaseFacts) { f.Subjects[0].Name = "/home/ali/out/nself" },
		"private address in the repo":  func(f *provenance.ReleaseFacts) { f.RepoID = "git@192.168.0.9:x/y" },
		"unknown visibility scrubs":    func(f *provenance.ReleaseFacts) { f.Visibility = ""; f.Pipeline = "10.9.8.7" },
	} {
		f := releaseFacts()
		mutate(&f)
		if _, err := provenance.Build(f); !errors.Is(err, provenance.ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestBuildRefusesBadFacts(t *testing.T) {
	for name, mutate := range map[string]func(*provenance.ReleaseFacts){
		"empty repo":          func(f *provenance.ReleaseFacts) { f.RepoID = "" },
		"empty pipeline":      func(f *provenance.ReleaseFacts) { f.Pipeline = "" },
		"empty job":           func(f *provenance.ReleaseFacts) { f.Job = "" },
		"empty isolation":     func(f *provenance.ReleaseFacts) { f.Isolation = "" },
		"empty version":       func(f *provenance.ReleaseFacts) { f.NselfCIVersion = "" },
		"empty coordinator":   func(f *provenance.ReleaseFacts) { f.Coordinator = "" },
		"empty runner":        func(f *provenance.ReleaseFacts) { f.Runner = "" },
		"empty run id":        func(f *provenance.ReleaseFacts) { f.RunID = "" },
		"empty attempt id":    func(f *provenance.ReleaseFacts) { f.AttemptID = "" },
		"control character":   func(f *provenance.ReleaseFacts) { f.Job = "a\nb" },
		"long field":          func(f *provenance.ReleaseFacts) { f.Job = strings.Repeat("j", 513) },
		"bad utf-8":           func(f *provenance.ReleaseFacts) { f.Job = "a\xffb" },
		"short revision":      func(f *provenance.ReleaseFacts) { f.Revision = "abc" },
		"upper-case revision": func(f *provenance.ReleaseFacts) { f.Revision = strings.Repeat("A", 40) },
		"bad config digest":   func(f *provenance.ReleaseFacts) { f.ConfigDigest = "zz" },
		"bad evidence digest": func(f *provenance.ReleaseFacts) { f.Evidence = hex64("G") },
		"no subjects":         func(f *provenance.ReleaseFacts) { f.Subjects = nil },
		"bad subject digest":  func(f *provenance.ReleaseFacts) { f.Subjects[0].SHA256 = "x" },
		"empty subject name":  func(f *provenance.ReleaseFacts) { f.Subjects[0].Name = "" },
		"repeated subject":    func(f *provenance.ReleaseFacts) { f.Subjects[1].Name = f.Subjects[0].Name },
		"bad lockfile digest": func(f *provenance.ReleaseFacts) { f.Lockfiles[0].SHA256 = "x" },
		"bad sbom digest":     func(f *provenance.ReleaseFacts) { f.SBOMs[0].SHA256 = "x" },
		"bad image digest":    func(f *provenance.ReleaseFacts) { f.Image.SHA256 = "x" },
		"no start time":       func(f *provenance.ReleaseFacts) { f.StartedOn = time.Time{} },
		"no finish time":      func(f *provenance.ReleaseFacts) { f.FinishedOn = time.Time{} },
		"finish before start": func(f *provenance.ReleaseFacts) { f.FinishedOn = f.StartedOn.Add(-time.Second) },
		"repeated sbom":       func(f *provenance.ReleaseFacts) { f.SBOMs[1].Name = f.SBOMs[0].Name },
	} {
		f := releaseFacts()
		mutate(&f)
		if _, err := provenance.Build(f); !errors.Is(err, provenance.ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestBuildAgent(t *testing.T) {
	b, err := provenance.BuildAgent(agentFacts())
	if err != nil {
		t.Fatal(err)
	}
	st, err := provenance.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	bd, rd := st.Predicate.BuildDefinition, st.Predicate.RunDetails
	if bd.BuildType != "https://nself.org/ci/buildtypes/agent/v1" {
		t.Fatalf("build type %q", bd.BuildType)
	}
	if rd.Builder.ID != "https://github.com/nself-org/plugins/.github/workflows/release-agent.yml@refs/tags/agent-v1.5.0" {
		t.Fatalf("builder %q", rd.Builder.ID)
	}
	if rd.Metadata.InvocationID != "https://github.com/nself-org/plugins/actions/runs/1234567890" || rd.Metadata.StartedOn != "2026-10-19T11:00:00Z" || rd.Metadata.FinishedOn != "2026-10-19T11:30:00Z" {
		t.Fatalf("metadata %+v", rd.Metadata)
	}
	if st.GitCommit() != strings.Repeat("b", 40) || st.SubjectDigests()["nself-ci-agent-linux-amd64"] != hex64("1") || st.Subject[0].Name != "nself-ci-agent-linux-amd64" {
		t.Fatalf("subjects or commit: %+v", st)
	}
	if d, ok := st.Byproduct("nself-ci-agent-linux-amd64.cdx.json"); !ok || d != hex64("3") {
		t.Fatal("sbom byproduct")
	}
	again, _ := provenance.BuildAgent(agentFacts())
	if !bytes.Equal(b, again) {
		t.Fatal("agent statement is not deterministic")
	}
	f := agentFacts()
	f.StartedOn, f.FinishedOn = time.Time{}, time.Time{}
	nb, err := provenance.BuildAgent(f)
	if err != nil {
		t.Fatal(err)
	}
	checkSchemas(t, nb)
	if s2, _ := provenance.Parse(nb); s2.Predicate.RunDetails.Metadata.StartedOn != "" {
		t.Fatal("times written without being set")
	}
}

func TestBuildAgentRefusesBadFacts(t *testing.T) {
	for name, mutate := range map[string]func(*provenance.AgentFacts){
		"repo":                func(f *provenance.AgentFacts) { f.Repo = "nself" },
		"repo with space":     func(f *provenance.AgentFacts) { f.Repo = "a b/c" },
		"ref":                 func(f *provenance.AgentFacts) { f.Ref = "" },
		"ref with space":      func(f *provenance.AgentFacts) { f.Ref = "refs/tags/a b" },
		"run id":              func(f *provenance.AgentFacts) { f.RunID = "12a" },
		"revision":            func(f *provenance.AgentFacts) { f.Revision = "xyz" },
		"no subjects":         func(f *provenance.AgentFacts) { f.Subjects = nil },
		"bad sbom":            func(f *provenance.AgentFacts) { f.SBOMs[0].SHA256 = "x" },
		"finish before start": func(f *provenance.AgentFacts) { f.FinishedOn = f.StartedOn.Add(-time.Second) },
		"only a start":        func(f *provenance.AgentFacts) { f.FinishedOn = time.Time{} },
		"only a finish":       func(f *provenance.AgentFacts) { f.StartedOn = time.Time{} },
	} {
		f := agentFacts()
		mutate(&f)
		if _, err := provenance.BuildAgent(f); !errors.Is(err, provenance.ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

// TestPublicationProfileGolden pins the exact bytes of a public fixture
// statement. UPDATE_GOLDEN=1 rewrites the files.
func TestPublicationProfileGolden(t *testing.T) {
	for name, build := range map[string]func() ([]byte, error){
		"testdata/golden/release-public.json": func() ([]byte, error) { return provenance.Build(releaseFacts()) },
		"testdata/golden/agent.json":          func() ([]byte, error) { return provenance.BuildAgent(agentFacts()) },
	} {
		got, err := build()
		if err != nil {
			t.Fatal(err)
		}
		if os.Getenv("UPDATE_GOLDEN") == "1" {
			if err = os.MkdirAll("testdata/golden", 0o755); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(name, append(got, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if string(bytes.TrimSuffix(want, []byte("\n"))) != string(got) {
			t.Errorf("%s differs:\n%s\n%s", name, want, got)
		}
	}
}

// TestLevelIsExactlyL1 pins the one claimed level (D14): the constant, the
// manifest's field and the statement carry no other.
func TestLevelIsExactlyL1(t *testing.T) {
	if provenance.Level != "L1" || manifest.SLSALevel != "L1" || provenance.PredicateType != "https://slsa.dev/provenance/v1" {
		t.Fatal("level or predicate type changed")
	}
	if strings.Contains(strings.ToLower(string(mustBuild(t, releaseFacts()))), "slsa_build_level") {
		t.Fatal("the statement must not restate the level; the manifest and verify output carry it")
	}
}
