package pipeline_test

// Purpose: Dispatch tests of P7-HOST-06: TestIdempotentDispatch, TestSpecLimit, the transport mode
// (inputs from 17.10, variables before), the ref choice, the request body and the refusals before any
// request is made.
// Inputs:  the fakeGL routes.
// Outputs: pass or fail.
// Constraints: no live network; no test sleeps.

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/pipeline"
	gitlabtmpl "github.com/nself-org/plugins/free/ci/templates/gitlab"
)

func createBody(t *testing.T, r recorded) (ref string, inputs map[string]string, vars map[string]string) {
	t.Helper()
	var b struct {
		Ref       string              `json:"ref"`
		Inputs    map[string]string   `json:"inputs"`
		Variables []map[string]string `json:"variables"`
	}
	if err := json.Unmarshal(r.Body, &b); err != nil {
		t.Fatal(err)
	}
	vars = map[string]string{}
	for _, v := range b.Variables {
		vars[v["key"]] = v["value"]
		if v["variable_type"] != "env_var" {
			t.Errorf("variable %s has type %q", v["key"], v["variable_type"])
		}
	}
	return b.Ref, b.Inputs, vars
}

// TestDispatchModes proves the Dispatcher passes the values as pipeline inputs on GitLab 17.10 and
// later and as the six NSELF_CI_* variables (plus the tag, the timeout and the fetch ref) before it or
// when the version cannot be read, on the default branch for trusted work, never through the trigger
// endpoint and never with the token in a URL or a body.
func TestDispatchModes(t *testing.T) {
	v15(t)
	for _, c := range []struct {
		name, version string
		inputs        bool
	}{{"18.2 takes inputs", "18.2.1-ee", true}, {"17.10 takes inputs", "17.10.0", true}, {"17.9 falls back", "17.9.3-ee", false},
		{"16.11 falls back", "16.11.2", false}, {"unreadable falls back", "", false}, {"nonsense falls back", "banana", false}} {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeGL(t)
			f.Version = c.version
			a := attemptFor("attempt-1", 1)
			a.FetchRef = "refs/merge-requests/12/head"
			h, err := f.provider(options{}).Dispatch(context.Background(), a)
			if err != nil {
				t.Fatal(err)
			}
			posts := f.requests("POST", "/pipeline")
			if len(posts) != 1 || h.RunID != "acme/widgets/9001" || h.Ref != "main" {
				t.Fatalf("posts=%d handle=%+v", len(posts), h)
			}
			ref, inputs, vars := createBody(t, posts[0])
			if ref != "main" {
				t.Errorf("ref = %q", ref)
			}
			if c.inputs {
				want := map[string]string{"revision": testRevision, "attempt": "attempt-1", "nonce": h.Nonce, "compat": "v1.5", "runner_tag": "saas-linux-small-amd64",
					"timeout_minutes": "30", "fetch_ref": "refs/merge-requests/12/head"}
				for k, v := range want {
					if inputs[k] != v {
						t.Errorf("input %s = %q, want %q", k, inputs[k], v)
					}
				}
				if len(vars) != 0 || inputs["spec_0"] == "" || inputs["spec"] != "" || !strings.HasPrefix(inputs["spec_digest"], "sha256:") {
					t.Errorf("inputs mode sent variables %v or lost the spec chunks: %v", vars, inputs)
				}
				return
			}
			want := map[string]string{"NSELF_CI_REVISION": testRevision, "NSELF_CI_ATTEMPT": "attempt-1", "NSELF_CI_NONCE": h.Nonce, "NSELF_CI_COMPAT": "v1.5",
				"NSELF_CI_RUNNER_TAG": "saas-linux-small-amd64", "NSELF_CI_TIMEOUT_MINUTES": "30", "NSELF_CI_FETCH_REF": "refs/merge-requests/12/head"}
			for k, v := range want {
				if vars[k] != v {
					t.Errorf("variable %s = %q, want %q", k, vars[k], v)
				}
			}
			if vars["NSELF_CI_SPEC"] == "" || !strings.HasPrefix(vars["NSELF_CI_SPEC_DIGEST"], "sha256:") || len(inputs) != 0 {
				t.Errorf("variables mode lost the spec or sent inputs: %v", inputs)
			}
		})
	}
}

// TestNoTriggerEndpoint proves the acceptance line "no request goes to the pipeline trigger endpoint" and that no
// token is in any URL, query or body, over a whole dispatch, collect and cancel.
func TestNoTriggerEndpoint(t *testing.T) {
	v15(t)
	f := newFakeGL(t)
	p := f.provider(options{})
	h, err := p.Dispatch(context.Background(), attemptFor("attempt-1", 1))
	if err != nil {
		t.Fatal(err)
	}
	f.finish(9001, "attempt-1", testRevision, nil)
	if _, err = p.Collect(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	_ = p.Cancel(context.Background(), h)
	for _, r := range f.Reqs {
		if strings.Contains(r.Path, "trigger") || strings.Contains(r.Path+r.Query+string(r.Body), testToken) {
			t.Errorf("request %s %s?%s leaks or triggers", r.Method, r.Path, r.Query)
		}
	}
}

// TestUntrustedRef proves untrusted work (and a trust class this build does not know) is dispatched on
// the untrusted ref without asking the project for its default branch, and that a bad ref is refused.
func TestUntrustedRef(t *testing.T) {
	v15(t)
	for _, trust := range []string{"untrusted", "", "brand-new-class"} {
		f := newFakeGL(t)
		a := attemptFor("attempt-1", 1)
		a.Trust = trust
		h, err := f.provider(options{}).Dispatch(context.Background(), a)
		if err != nil || h.Ref != "nself-ci/untrusted" {
			t.Fatalf("trust %q: %+v %v", trust, h, err)
		}
		if len(f.requests("GET", "/projects/acme%2Fwidgets")) != 0 && len(f.requests("", "default")) != 0 {
			t.Errorf("trust %q: the default branch was looked up", trust)
		}
		ref, _, _ := createBody(t, f.requests("POST", "/pipeline")[0])
		if ref != "nself-ci/untrusted" {
			t.Errorf("trust %q: ref %q", trust, ref)
		}
	}
	f := newFakeGL(t)
	a := attemptFor("attempt-1", 1)
	a.Trust = "untrusted"
	_, err := f.provider(options{UntrustedRef: "refs/heads/x"}).Dispatch(context.Background(), a)
	wantCode(t, err, "E702", "untrusted_ref")
	f.DefaultBranch = "../evil"
	_, err = f.provider(options{}).Dispatch(context.Background(), attemptFor("attempt-2", 1))
	wantCode(t, err, "E702", "default_branch")
}

// TestRefusalsBeforeRequests proves anything outside the allowlists is E708 before any request is made.
func TestRefusalsBeforeRequests(t *testing.T) {
	v15(t)
	mutate := []struct {
		reason string
		mut    func(*providers.Attempt)
	}{
		{"repo", func(a *providers.Attempt) { a.Repo = "widgets" }},
		{"repo", func(a *providers.Attempt) { a.Repo = "acme/../x" }},
		{"attempt", func(a *providers.Attempt) { a.ID = "a\"b" }},
		{"attempt", func(a *providers.Attempt) { a.ID = "" }},
		{"revision", func(a *providers.Attempt) { a.Revision = "main" }},
		{"fetch_ref", func(a *providers.Attempt) { a.FetchRef = "refs/pull/1/head" }},
		{"label.not_allowed", func(a *providers.Attempt) { a.Label = "saas-linux-large-amd64" }},
		{"compat_mode", func(a *providers.Attempt) { a.CompatMode = "v9" }},
		{"timeout.over_360", func(a *providers.Attempt) { a.TimeoutMs = 7 * 60 * 60 * 1000 }},
		{"spec.digest", func(a *providers.Attempt) { a.SpecDigest = "sha256:" + strings.Repeat("0", 64) }},
	}
	for _, c := range mutate {
		reason := c.reason
		f := newFakeGL(t)
		a := attemptFor("attempt-1", 1)
		c.mut(&a)
		_, err := f.provider(options{}).Dispatch(context.Background(), a)
		wantCode(t, err, "E708", reason)
		if len(f.Reqs) != 0 {
			t.Errorf("%s: %d requests were made", reason, len(f.Reqs))
		}
	}
	t.Setenv("NSELF_V15", "")
	_, err := newFakeGL(t).provider(options{}).Dispatch(context.Background(), attemptFor("attempt-1", 1))
	wantCode(t, err, "E708", "hosted dispatch needs v1.5")
}

// TestSpecLimit proves acceptance line "an oversize spec yields E708": the encoded spec may hold
// MaxSpecChars characters (the documented limit of a pipeline variable value, recorded from
// https://docs.gitlab.com/ci/variables/ on 2026-10-19) and one more is refused before any request;
// a project that refuses pipeline variables yields E702 with the remediation.
func TestSpecLimit(t *testing.T) {
	v15(t)
	if pipeline.MaxSpecChars != 10000 {
		t.Fatalf("MaxSpecChars = %d, the documented limit is 10,000 characters", pipeline.MaxSpecChars)
	}
	big := func(n int) []byte { // random-looking bytes do not compress: the encoded size tracks n
		b := make([]byte, n)
		x := uint32(2463534242)
		for i := range b {
			x ^= x << 13
			x ^= x >> 17
			x ^= x << 5
			b[i] = byte(x)
		}
		return b
	}
	f := newFakeGL(t)
	a := attemptFor("attempt-1", 1)
	a.Spec = big(8000) // 8000 random bytes are about 10,700 characters of base64
	_, err := f.provider(options{}).Dispatch(context.Background(), a)
	wantCode(t, err, "E708", "spec.too_large")
	if len(f.Reqs) != 0 {
		t.Errorf("an oversize spec made %d requests", len(f.Reqs))
	}
	a.Spec = big(7000) // about 9,400 characters: inside the limit
	f = newFakeGL(t)
	if _, err = f.provider(options{}).Dispatch(context.Background(), a); err != nil {
		t.Fatalf("a spec inside the limit: %v", err)
	}
	_, inputs, _ := createBody(t, f.requests("POST", "/pipeline")[0])
	joined := ""
	for i := 0; i < 10; i++ {
		joined += inputs["spec_"+strconv.Itoa(i)]
	}
	if n := len(joined); n == 0 || n > pipeline.MaxSpecChars {
		t.Errorf("sent spec length %d", n)
	}

	f = newFakeGL(t)
	f.Version, f.CreateMode = "16.11.2", "400"
	_, err = f.provider(options{}).Dispatch(context.Background(), attemptFor("attempt-2", 1))
	wantCode(t, err, "E702", pipeline.RemediationVariables)
	f = newFakeGL(t)
	f.CreateMode = "400" // inputs mode: a 400 is a plain E702, not the variables remediation
	_, err = f.provider(options{}).Dispatch(context.Background(), attemptFor("attempt-3", 1))
	wantCode(t, err, "E702", "request")
}

// TestIdempotentDispatch proves acceptance line 7: a create that times out after GitLab created the
// pipeline leads to adoption of that pipeline (one pipeline total), a repeated attempt number adopts the
// first pipeline instead of creating another, a nonce echoed by two pipelines or by a pipeline of
// another user yields E705 run.ambiguous, and a retry cancels the pipeline it replaces.
func TestIdempotentDispatch(t *testing.T) {
	v15(t)
	ctx := context.Background()
	for _, version := range []string{"18.2.1-ee", "16.11.2"} {
		t.Run("create then 502 on "+version, func(t *testing.T) {
			f := newFakeGL(t)
			f.Version, f.CreateMode = version, "502-exists"
			h, err := f.provider(options{}).Dispatch(ctx, attemptFor("attempt-1", 1))
			if err != nil || h.RunID != "acme/widgets/9001" || len(f.Pipes) != 1 || len(f.requests("POST", "/pipeline")) != 1 {
				t.Fatalf("handle=%+v err=%v pipes=%d posts=%d", h, err, len(f.Pipes), len(f.requests("POST", "/pipeline")))
			}
		})
	}
	t.Run("repeated number adopts", func(t *testing.T) {
		f := newFakeGL(t)
		p := f.provider(options{})
		first, err := p.Dispatch(ctx, attemptFor("attempt-1", 1))
		if err != nil {
			t.Fatal(err)
		}
		again, err := p.Dispatch(ctx, attemptFor("attempt-1", 1))
		if err != nil || again != first || len(f.Pipes) != 1 || len(f.requests("POST", "/pipeline")) != 1 {
			t.Fatalf("again=%+v first=%+v err=%v pipes=%d", again, first, err, len(f.Pipes))
		}
	})
	t.Run("nothing created, then a clear failure", func(t *testing.T) {
		f := newFakeGL(t)
		f.CreateMode = "502-none"
		_, err := f.provider(options{Window: 10 * time.Second}).Dispatch(ctx, attemptFor("attempt-1", 1))
		if err == nil || len(f.Pipes) != 0 || len(f.requests("POST", "/pipeline")) != 1 {
			t.Fatalf("err=%v pipes=%d posts=%d", err, len(f.Pipes), len(f.requests("POST", "/pipeline")))
		}
	})
	t.Run("two pipelines echo one nonce", func(t *testing.T) {
		f := newFakeGL(t)
		f.CreateMode = "502-exists"
		f.Extra = func(first *fakePipe) {
			if first.ID == 9001 { // a second pipeline by the same user with the same nonce
				f.NextID++
				twin := *first
				twin.ID = f.NextID
				f.Pipes[twin.ID] = &twin
			}
		}
		_, err := f.provider(options{}).Dispatch(ctx, attemptFor("attempt-1", 1))
		wantCode(t, err, "E705", "run.ambiguous")
	})
	t.Run("a planted pipeline of another user", func(t *testing.T) {
		f := newFakeGL(t)
		f.CreateMode = "502-exists"
		f.Extra = func(first *fakePipe) {
			if first.ID == 9001 {
				f.NextID++
				planted := *first
				planted.ID, planted.User = f.NextID, "intruder"
				f.Pipes[planted.ID] = &planted
			}
		}
		_, err := f.provider(options{}).Dispatch(ctx, attemptFor("attempt-1", 1))
		wantCode(t, err, "E705", "run.ambiguous")
	})
	t.Run("a retry cancels the pipeline it replaces", func(t *testing.T) {
		f := newFakeGL(t)
		p := f.provider(options{})
		first, err := p.Dispatch(ctx, attemptFor("attempt-1", 1))
		if err != nil {
			t.Fatal(err)
		}
		second, err := p.Dispatch(ctx, attemptFor("attempt-1-retry-1", 1))
		if err != nil || second.RunID == first.RunID || second.Nonce == first.Nonce {
			t.Fatalf("second=%+v first=%+v err=%v", second, first, err)
		}
		if f.Pipes[9001].Cancelled != 1 || f.Pipes[9001].Status != "canceled" {
			t.Errorf("the replaced pipeline was not cancelled: %+v", f.Pipes[9001])
		}
		f.finish(9001, "attempt-1", testRevision, nil)
		_, err = p.Collect(ctx, first)
		wantCode(t, err, "E705", "attempt.superseded")
	})
	t.Run("a lower number is superseded", func(t *testing.T) {
		f := newFakeGL(t)
		p := f.provider(options{})
		if _, err := p.Dispatch(ctx, attemptFor("attempt-1", 3)); err != nil {
			t.Fatal(err)
		}
		_, err := p.Dispatch(ctx, attemptFor("attempt-1", 2))
		wantCode(t, err, "E708", "attempt.superseded")
	})
}

// TestInputsNeedTheMainFile proves the inputs transport is chosen only when the instance takes inputs
// AND the main .gitlab-ci.yml at the dispatch ref declares every input (GitLab refuses inputs the main
// file does not declare and an included file cannot receive them): a missing main file, a main file
// without a spec header and one that declares only some inputs all fall back to variables.
func TestInputsNeedTheMainFile(t *testing.T) {
	v15(t)
	partial := []byte("spec:\n  inputs:\n    nonce:\n      default: ''\n---\nvariables:\n  X: y\n")
	for name, root := range map[string][]byte{"declares all": rootFile(), "missing": nil, "no spec header": []byte("variables:\n  X: y\n"),
		"declares some": partial, "not yaml": []byte("a: [unclosed")} {
		f := newFakeGL(t)
		f.Root = root
		if _, err := f.provider(options{}).Dispatch(context.Background(), attemptFor("attempt-1", 1)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		_, inputs, vars := createBody(t, f.requests("POST", "/pipeline")[0])
		if wantInputs := name == "declares all"; wantInputs != (len(inputs) > 0) || wantInputs == (len(vars) > 0) {
			t.Errorf("%s: inputs=%d variables=%d", name, len(inputs), len(vars))
		}
	}
}

// TestInputChunks proves a spec longer than one input's 1 KB limit is split into spec_N chunks of at
// most ChunkChars characters, no more than SpecChunks of them, that join back to exactly the spec the
// variables transport sends, and that the inputs stay inside GitLab's limit of 20 per pipeline.
func TestInputChunks(t *testing.T) {
	v15(t)
	a := attemptFor("attempt-1", 1)
	x := uint32(7)
	a.Spec = make([]byte, 5000)
	for i := range a.Spec {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		a.Spec[i] = byte(x)
	}
	fi, fv := newFakeGL(t), newFakeGL(t)
	fv.Root = nil
	if _, err := fi.provider(options{}).Dispatch(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if _, err := fv.provider(options{}).Dispatch(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	_, inputs, _ := createBody(t, fi.requests("POST", "/pipeline")[0])
	_, _, vars := createBody(t, fv.requests("POST", "/pipeline")[0])
	joined, chunks := "", 0
	for i := 0; i < 10; i++ {
		c := inputs["spec_"+strconv.Itoa(i)]
		if len(c) > 1000 {
			t.Errorf("chunk %d has %d characters", i, len(c))
		}
		if c != "" {
			chunks++
		}
		joined += c
	}
	if chunks < 6 || joined != vars["NSELF_CI_SPEC"] || len(inputs) > 20 {
		t.Errorf("chunks=%d inputs=%d joined==variables spec: %v", chunks, len(inputs), joined == vars["NSELF_CI_SPEC"])
	}
}

// TestCreate400IsDiagnosed proves a 400 from the create call is not always blamed on the pipeline
// variables: when the ref does not exist the answer says so (the untrusted ref was never created),
// otherwise variables mode gets the remediation and inputs mode a plain E702.
func TestCreate400IsDiagnosed(t *testing.T) {
	v15(t)
	f := newFakeGL(t)
	f.Version, f.CreateMode, f.BranchSt = "16.11.2", "400", 404
	f.Root = gitlabtmpl.RenderRoot(gitlabtmpl.RootVariables) // an older instance: the untrusted ref carries the variables root
	a := attemptFor("attempt-1", 1)
	a.Trust = "untrusted"
	_, err := f.provider(options{}).Dispatch(context.Background(), a)
	var pe *providers.Error
	if !errors.As(err, &pe) || pe.Code != "E702" || !strings.HasPrefix(pe.Reason, "ref.missing: create the branch nself-ci/untrusted") {
		t.Fatalf("missing ref: %v", err)
	}
	f = newFakeGL(t)
	f.Version, f.CreateMode = "16.11.2", "400"
	_, err = f.provider(options{}).Dispatch(context.Background(), attemptFor("attempt-2", 1))
	wantCode(t, err, "E702", pipeline.RemediationVariables)
}

// TestJournalFences proves the dispatch journal refuses what would run the wrong pipeline: a second
// dispatch of one attempt number with another revision or spec (attempt.conflict), a late dispatch of an
// earlier try after a later retry began (it must not cancel the retry), and an attempt number below 1.
func TestJournalFences(t *testing.T) {
	v15(t)
	ctx := context.Background()
	f := newFakeGL(t)
	p := f.provider(options{})
	if _, err := p.Dispatch(ctx, attemptFor("job-1", 1)); err != nil {
		t.Fatal(err)
	}
	other := attemptFor("job-1", 1)
	other.Revision = strings.Repeat("a", 40)
	_, err := p.Dispatch(ctx, other)
	wantCode(t, err, "E708", "attempt.conflict")
	spec := attemptFor("job-1", 1)
	spec.Spec = []byte(`{"kind":"other"}`)
	_, err = p.Dispatch(ctx, spec)
	wantCode(t, err, "E708", "attempt.conflict")

	f = newFakeGL(t)
	p = f.provider(options{})
	first, err := p.Dispatch(ctx, attemptFor("job-1", 1))
	if err != nil {
		t.Fatal(err)
	}
	retry, err := p.Dispatch(ctx, attemptFor("job-1-retry-2", 1))
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Dispatch(ctx, attemptFor("job-1", 5)) // the first try, delivered late
	wantCode(t, err, "E708", "attempt.superseded")
	if f.Pipes[retryID(t, retry)].Cancelled != 0 {
		t.Error("the late dispatch of the first try cancelled the retry's pipeline")
	}
	if f.Pipes[retryID(t, first)].Cancelled != 1 {
		t.Error("the first try's pipeline was not cancelled by its retry")
	}
	for _, n := range []int{0, -3} {
		_, err = p.Dispatch(ctx, attemptFor("job-9", n))
		wantCode(t, err, "E708", "attempt.number")
	}
}

func retryID(t *testing.T, h providers.Handle) int64 {
	t.Helper()
	i := strings.LastIndex(h.RunID, "/")
	n, err := strconv.ParseInt(h.RunID[i+1:], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TestTooManyCandidates proves a crowded ref is a retryable E704 and never a silent cut: with more
// pipelines on the ref than the search reads, the lookup refuses to guess.
func TestTooManyCandidates(t *testing.T) {
	v15(t)
	f := newFakeGL(t)
	f.CreateMode = "502-exists"
	f.Extra = func(first *fakePipe) {
		for i := 0; i < 120; i++ {
			f.NextID++
			twin := *first
			twin.ID, twin.Name, twin.User = f.NextID, "other work", "someone"
			f.Pipes[twin.ID] = &twin
		}
	}
	_, err := f.provider(options{Window: 5 * time.Second}).Dispatch(context.Background(), attemptFor("attempt-1", 1))
	wantCode(t, err, "E704", "run.candidates")
}

// TestUntrustedRefMustBeGenerated proves untrusted work dispatches only when the untrusted ref carries
// exactly one of the two generated root files: an edited root, a missing one and an inputs root on an
// instance older than 17.10 are E702 before anything is created; the variables root on a new instance
// gives the variables transport and the inputs root the inputs transport. The ref is writable by every
// project member, so the root is what decides how the values reach the job (review F1).
func TestUntrustedRefMustBeGenerated(t *testing.T) {
	v15(t)
	ctx := context.Background()
	untrusted := func() providers.Attempt {
		a := attemptFor("attempt-1", 1)
		a.Trust = "untrusted"
		return a
	}
	edited := append(gitlabtmpl.RenderRoot(gitlabtmpl.RootInputs), []byte("variables:\n  NSELF_CI_SPEC_DIGEST: sha256:forged\n")...)
	for name, c := range map[string]struct {
		root    []byte
		version string
		reason  string // empty: dispatch works
		mode    string
	}{
		"edited root":                  {edited, "18.2.1-ee", pipeline.ReasonDrifted, ""},
		"project's own root":           {[]byte("include:\n  - local: other.yml\n"), "18.2.1-ee", pipeline.ReasonDrifted, ""},
		"no root":                      {nil, "18.2.1-ee", pipeline.ReasonDrifted, ""},
		"inputs root on an old GitLab": {gitlabtmpl.RenderRoot(gitlabtmpl.RootInputs), "17.9.3", "untrusted_ref.needs_inputs: the ref carries the inputs root but this GitLab is older than 17.10", ""},
		"inputs root on a new GitLab":  {gitlabtmpl.RenderRoot(gitlabtmpl.RootInputs), "18.2.1-ee", "", pipeline.ModeInputs},
		"variables root on a new one":  {gitlabtmpl.RenderRoot(gitlabtmpl.RootVariables), "18.2.1-ee", "", pipeline.ModeVariables},
		"variables root on an old one": {gitlabtmpl.RenderRoot(gitlabtmpl.RootVariables), "15.11.13", "", pipeline.ModeVariables},
	} {
		f := newFakeGL(t)
		f.Root, f.Version = c.root, c.version
		_, err := f.provider(options{}).Dispatch(ctx, untrusted())
		posts := f.requests("POST", "/pipeline")
		if c.reason != "" {
			wantCode(t, err, "E702", c.reason)
			if len(posts) != 0 {
				t.Errorf("%s: %d pipelines were created", name, len(posts))
			}
			continue
		}
		if err != nil || len(posts) != 1 {
			t.Fatalf("%s: %v posts=%d", name, err, len(posts))
		}
		_, inputs, vars := createBody(t, posts[0])
		if (c.mode == pipeline.ModeInputs) != (len(inputs) > 0) || (c.mode == pipeline.ModeVariables) != (len(vars) > 0) {
			t.Errorf("%s: inputs=%d variables=%d, want %s", name, len(inputs), len(vars), c.mode)
		}
	}
}
