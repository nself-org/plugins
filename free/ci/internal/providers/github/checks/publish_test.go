package checks

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

var (
	keyOnce sync.Once
	theKey  *rsa.PrivateKey
	thePEM  []byte
)

func testKey(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	keyOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		theKey, thePEM = k, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
	})
	return theKey, thePEM
}

// dashes is built apart so no source line holds a literal PEM marker (secret scanners flag it).
const dashes = "-----"

var t0 = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func newPublisher(t *testing.T, f *fakeGH, now func() time.Time) *Publisher {
	t.Helper()
	_, p := testKey(t)
	pub, err := New(Config{AppID: "12345", InstallationID: "777", KeyEnv: "X"}, p, f.factory(), now)
	if err != nil {
		t.Fatal(err)
	}
	return pub
}

func fixedNow() time.Time { return t0 }

func failingEvidence() model.Evidence {
	return fixtureEvidence(
		check("lint", "pass", nil),
		check("unit-tests", "fail", func(c *model.Check) {
			c.FailureClass = "code"
			c.Findings = 2
			c.Diagnosis = "code"
			c.Remediation = "Fix the failing tests | then rerun"
			c.Excerpt = "--- FAIL: TestX\nfree/ci/internal/x/x.go:42:7: undefined: y\n/etc/passwd:1: nope\n../up.go:3: no\nplain line"
		}),
		check("docs", "skip", nil),
	)
}

// TestCheckRunGateTableGolden: the rendered table is byte-stable, and a failing run creates one
// check run named nself-ci/gates with the table, the located annotation and conclusion failure.
func TestCheckRunGateTableGolden(t *testing.T) {
	r := request(report.Failure, failingEvidence())
	got := Table(r.Evidence, r.Projection)
	golden := "testdata/gate-table-fail.golden.md"
	if os.Getenv("NSELF_UPDATE_GOLDEN") == "1" {
		_ = os.WriteFile(golden, []byte(got), 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil || string(want) != got {
		t.Fatalf("gate table differs from %s (NSELF_UPDATE_GOLDEN=1 rewrites it):\n%s", golden, got)
	}

	key, _ := testKey(t)
	f := newFake(t, key)
	p := newPublisher(t, f, fixedNow)
	if err := p.Detail(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if f.count("POST", pathCreate) != 1 || f.count("GET", pathList) != 1 || f.count("PATCH", pathUpdate) != 0 {
		t.Fatalf("calls: %+v", f.snapshot())
	}
	var body checkBody
	for _, c := range f.snapshot() {
		if c.Path == pathCreate {
			if c.Bearer != testToken {
				t.Fatal("check run created with something other than the installation token")
			}
			if err := json.Unmarshal(c.Body, &body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if body.Name != "nself-ci/gates" || body.Name == report.Context || body.HeadSHA != testSHA || body.ExternalID != "run-0001" ||
		body.Status != "completed" || body.Conclusion != "failure" || body.CompletedAt != "2026-10-20T12:00:00Z" {
		t.Fatalf("body: %+v", body)
	}
	if body.Output == nil || body.Output.Text != got || len(body.Output.Annotations) != 1 {
		t.Fatalf("output: %+v", body.Output)
	}
	a := body.Output.Annotations[0]
	if a.Path != "free/ci/internal/x/x.go" || a.Start != 42 || a.Level != "failure" || a.Message != "undefined: y" || a.Title != "unit-tests" {
		t.Fatalf("annotation: %+v", a)
	}
}

// TestConclusionMapping: the check run follows the status of record. pass -> success; status failure
// -> failure; status error (infra error, lost runner, timeout) -> neutral; a failure whose every
// unsuccessful required check was cancelled, or a cancelled run -> cancelled; pending -> none.
// The pairs are the ones a real run produces (run.TestChecksConclusionFollowsRunResult feeds the
// real evidence builder through this function).
func TestConclusionMapping(t *testing.T) {
	cancelled := func(c *model.Check) { c.FailureClass = "cancelled" }
	code := func(c *model.Check) { c.FailureClass = "code" }
	cases := []struct {
		name   string
		state  report.State
		result model.Result
		checks []model.Check
		want   Conclusion
	}{
		{"pass", report.Success, "pass", []model.Check{check("a", "pass", nil)}, Success},
		{"code failure", report.Failure, "fail", []model.Check{check("a", "fail", code)}, Failure},
		{"unclassified failure", report.Failure, "fail", []model.Check{check("a", "fail", nil)}, Failure},
		{"skip plus error stays a failure", report.Failure, "fail", []model.Check{check("a", "skip", nil), check("b", "error", nil)}, Failure},
		{"infra error status", report.Error, "error", []model.Check{check("a", "error", func(c *model.Check) { c.FailureClass = "infra" })}, Neutral},
		{"lost status", report.Error, "error", []model.Check{check("a", "error", func(c *model.Check) { c.FailureClass = "lost" })}, Neutral},
		{"cancelled result", report.Failure, "cancelled", []model.Check{check("a", "error", cancelled)}, Cancelled},
		{"cancelled class only", report.Failure, "fail", []model.Check{check("a", "fail", cancelled), check("o", "fail", func(c *model.Check) { c.Required = false })}, Cancelled},
		{"cancelled and code", report.Failure, "fail", []model.Check{check("a", "fail", cancelled), check("b", "fail", code)}, Failure},
		{"success status over failing run", report.Success, "fail", []model.Check{check("a", "fail", code)}, Failure},
		{"pending", report.Pending, "", nil, ""},
	}
	for _, c := range cases {
		ev := fixtureEvidence(c.checks...)
		ev.Result = c.result
		if got := ConclusionFor(c.state, ev); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

// TestPassingRunSuccess: a passing run concludes success and sends no annotation.
func TestPassingRunSuccess(t *testing.T) {
	key, _ := testKey(t)
	f := newFake(t, key)
	r := request(report.Success, fixtureEvidence(check("a", "pass", nil)))
	if err := newPublisher(t, f, fixedNow).Detail(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	var body checkBody
	for _, c := range f.snapshot() {
		if c.Path == pathCreate {
			_ = json.Unmarshal(c.Body, &body)
		}
	}
	if body.Conclusion != "success" || len(body.Output.Annotations) != 0 {
		t.Fatalf("body: %+v", body)
	}
}

// TestRepeatUpdatesNotDuplicates: a second publication for the same (revision, run) updates the
// existing check run and does not send the annotations again.
func TestRepeatUpdatesNotDuplicates(t *testing.T) {
	key, _ := testKey(t)
	f := newFake(t, key)
	f.list = "list-existing.json"
	if err := newPublisher(t, f, fixedNow).Detail(context.Background(), request(report.Failure, failingEvidence())); err != nil {
		t.Fatal(err)
	}
	if f.count("POST", pathCreate) != 0 || f.count("PATCH", pathUpdate) != 1 {
		t.Fatalf("calls: %+v", f.snapshot())
	}
	for _, c := range f.snapshot() {
		if c.Path == pathUpdate {
			if c.Params["id"] != "9002" || strings.Contains(string(c.Body), `"annotations"`) {
				t.Fatalf("update: %+v %s", c.Params, c.Body)
			}
		}
	}
}

// TestPendingPublishesNothing: a pending status has no conclusion, so no call is made.
func TestPendingPublishesNothing(t *testing.T) {
	key, _ := testKey(t)
	f := newFake(t, key)
	r := request(report.Failure, failingEvidence())
	r.Status.State = report.Pending
	if err := newPublisher(t, f, fixedNow).Detail(context.Background(), r); err != nil || len(f.snapshot()) != 0 {
		t.Fatalf("err %v calls %+v", err, f.snapshot())
	}
}

// TestAnnotationPaging: 50 per request, in pages, capped at MaxAnnotations in all.
func TestAnnotationPaging(t *testing.T) {
	for _, tc := range []struct{ located, want int }{{3, 3}, {50, 50}, {120, 120}, {260, MaxAnnotations}} {
		var lines []string
		for i := 1; i <= tc.located; i++ {
			lines = append(lines, fmt.Sprintf("pkg/f%d.go:%d:1: problem %d", i%7, i, i))
		}
		ev := fixtureEvidence(check("lint", "fail", func(c *model.Check) { c.Excerpt = strings.Join(lines, "\n") }))
		key, _ := testKey(t)
		f := newFake(t, key)
		if err := newPublisher(t, f, fixedNow).Detail(context.Background(), request(report.Failure, ev)); err != nil {
			t.Fatal(err)
		}
		total, requests := 0, 0
		for _, c := range f.snapshot() {
			if c.Path != pathCreate && c.Path != pathUpdate {
				continue
			}
			var b checkBody
			_ = json.Unmarshal(c.Body, &b)
			n := len(b.Output.Annotations)
			if n > 50 || PageSize != 50 { // the literal: GitHub accepts 50 annotations per request
				t.Fatalf("%d located: a request carried %d annotations", tc.located, n)
			}
			total += n
			requests++
		}
		wantRequests := (tc.want + PageSize - 1) / PageSize
		if total != tc.want || requests != max(1, wantRequests) {
			t.Fatalf("%d located: sent %d in %d requests, want %d in %d", tc.located, total, requests, tc.want, max(1, wantRequests))
		}
		if f.count("POST", pathCreate) != 1 {
			t.Fatalf("one check run expected, calls %+v", f.snapshot())
		}
	}
}

// TestInstallationTokenRefresh: the token is exchanged once, reused, and replaced 60 s before it expires.
func TestInstallationTokenRefresh(t *testing.T) {
	key, _ := testKey(t)
	f := newFake(t, key)
	clock := t0
	p := newPublisher(t, f, func() time.Time { return clock })
	ctx := context.Background()
	get := func() string {
		tok, err := p.tokens("widgets").Token(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	exchanges := func() int { return f.count("POST", "/app/installations/{id}/access_tokens") }
	if get() != testToken || exchanges() != 1 {
		t.Fatalf("first exchange: %d", exchanges())
	}
	clock = time.Date(2026, 10, 20, 12, 58, 59, 0, time.UTC) // 1 s before the 60 s margin of the 13:00 expiry
	get()
	if exchanges() != 1 {
		t.Fatalf("a token with more than 60 s left must be reused, exchanges %d", exchanges())
	}
	clock = time.Date(2026, 10, 20, 12, 59, 0, 0, time.UTC) // the margin starts here
	get()
	if exchanges() != 2 {
		t.Fatalf("a token inside the 60 s margin must be replaced, exchanges %d", exchanges())
	}
	// every exchange sent the four permissions and no others
	for _, c := range f.snapshot() {
		if c.Path == "/app/installations/{id}/access_tokens" {
			var b struct{ Permissions map[string]string }
			_ = json.Unmarshal(c.Body, &b)
			if len(b.Permissions) != 4 || b.Permissions["checks"] != "write" || b.Permissions["statuses"] != "write" || b.Permissions["contents"] != "read" || b.Permissions["metadata"] != "read" {
				t.Fatalf("permissions: %v", b.Permissions)
			}
			var rb struct{ Repositories []string }
			_ = json.Unmarshal(c.Body, &rb)
			if len(rb.Repositories) != 1 || rb.Repositories[0] != "widgets" {
				t.Fatalf("the token must be limited to the one repository: %v", rb.Repositories)
			}
			if c.Params["id"] != "777" {
				t.Fatalf("installation: %v", c.Params)
			}
		}
	}
	// no usable expiry: a short default lifetime, not forever
	f2 := newFake(t, key)
	f2.expires = "2020-01-01T00:00:00Z"
	clock = t0
	p2 := newPublisher(t, f2, func() time.Time { return clock })
	_, _ = p2.tokens("widgets").Token(ctx)
	clock = t0.Add(defaultLifetime)
	_, _ = p2.tokens("widgets").Token(ctx)
	if f2.count("POST", "/app/installations/{id}/access_tokens") != 2 {
		t.Fatal("an already-expired expiry must not be cached as valid")
	}
}

// TestNoSecretMaterialLeaks: neither the PEM, the JWT nor the installation token appears in an
// error, in the table, in an annotation or in any request body, even when GitHub echoes them back.
func TestNoSecretMaterialLeaks(t *testing.T) {
	key, p := testKey(t)
	pemBody := strings.Split(strings.TrimSpace(string(p)), "\n")[2]
	leaky := fixtureEvidence(check("unit-tests", "fail", func(c *model.Check) {
		c.Diagnosis = "token " + testToken
		c.Excerpt = "pkg/a.go:3:1: saw " + testToken + " and " + pemBody
	}))

	f := newFake(t, key)
	pub := newPublisher(t, f, fixedNow)
	if err := pub.Detail(context.Background(), request(report.Failure, leaky)); err != nil {
		t.Fatal(err)
	}
	var jwt string
	for _, c := range f.snapshot() {
		if c.Path == "/app/installations/{id}/access_tokens" {
			jwt = c.Bearer
		}
		for _, secret := range []string{testToken, pemBody, "PRIVATE KEY"} {
			if strings.Contains(string(c.Body), secret) {
				t.Fatalf("%s %s sent secret material %q in its body", c.Method, c.Path, secret[:6])
			}
		}
	}
	if jwt == "" {
		t.Fatal("no JWT seen")
	}

	// errors that echo the JWT or the token are masked
	for _, path := range []string{"/app/installations/{id}/access_tokens", pathList, pathCreate} {
		f := newFake(t, key)
		pub := newPublisher(t, f, fixedNow)
		f.failPath = path
		// the failure text carries what GitHub (or a proxy) might echo; the JWT is only known once signed
		f.failErr = fmt.Errorf("denied for Bearer %s and %s", testToken, pemBody)
		err := pub.Detail(context.Background(), request(report.Failure, failingEvidence()))
		if err == nil {
			t.Fatalf("%s: want an error", path)
		}
		if !strings.HasPrefix(err.Error(), "E816:") || strings.Contains(err.Error(), testToken) || strings.Contains(err.Error(), pemBody) {
			t.Fatalf("%s: error leaks or lacks the code: %v", path, err)
		}
	}

	// a JWT echoed back by the exchange error is masked (registered once signed)
	f3 := newFake(t, key)
	pub3 := newPublisher(t, f3, fixedNow)
	f3.failPath = "/app/installations/{id}/access_tokens"
	f3.failErr = jwtEcho{f3}
	err := pub3.Detail(context.Background(), request(report.Failure, failingEvidence()))
	if err == nil || strings.Contains(err.Error(), "eyJ") {
		t.Fatalf("a JWT echoed in an error must be masked: %v", err)
	}

	// the key text never reaches a parse error either
	if _, err := ParseKey([]byte(dashes + "BEGIN RSA PRIVATE KEY" + dashes + "\nAAAA\n" + dashes + "END RSA PRIVATE KEY" + dashes + "\n")); err == nil || strings.Contains(err.Error(), "AAAA") {
		t.Fatalf("parse error: %v", err)
	}
}

// jwtEcho is an error whose text is the Bearer JWT of the last exchange, like a server that echoes headers.
type jwtEcho struct{ f *fakeGH }

func (e jwtEcho) Error() string {
	for _, c := range e.f.snapshot() {
		if c.Bearer != "" {
			return "rejected " + c.Bearer
		}
	}
	return "rejected"
}

// TestJWTShape pins the RS256 header and that the signature is deterministic for a key and time.
func TestJWTShape(t *testing.T) {
	key, _ := testKey(t)
	a, err := SignJWT("12345", key, t0)
	b, _ := SignJWT("12345", key, t0)
	if err != nil || a != b || !strings.HasPrefix(a, "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.") {
		t.Fatalf("jwt: %v %q", err, a[:40])
	}
	c, _ := SignJWT("12345", key, t0.Add(time.Second))
	if c == a {
		t.Fatal("a later time must sign differently")
	}
}

func TestParseKeyForms(t *testing.T) {
	key, p := testKey(t)
	if k, err := ParseKey(p); err != nil || k.N.Cmp(key.N) != 0 {
		t.Fatalf("pkcs1: %v", err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	if k, err := ParseKey(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})); err != nil || k.N.Cmp(key.N) != 0 {
		t.Fatalf("pkcs8: %v", err)
	}
	for _, bad := range []string{"", "not pem", dashes + "BEGIN CERTIFICATE" + dashes + "\nAAAA\n" + dashes + "END CERTIFICATE" + dashes + "\n"} {
		if _, err := ParseKey([]byte(bad)); err == nil {
			t.Fatalf("%q must not parse", bad)
		}
	}
}

// TestRetryCompletesMissingAnnotations: a check run that already holds some annotations (an earlier
// page failed) receives only the rest, and the table text rides on every page.
func TestRetryCompletesMissingAnnotations(t *testing.T) {
	var lines []string
	for i := 1; i <= 120; i++ {
		lines = append(lines, fmt.Sprintf("pkg/f.go:%d:1: problem %d", i, i))
	}
	ev := fixtureEvidence(check("lint", "fail", func(c *model.Check) { c.Excerpt = strings.Join(lines, "\n") }))
	key, _ := testKey(t)
	f := newFake(t, key)
	f.list = "list-existing.json" // run-0001 holds 3 annotations
	if err := newPublisher(t, f, fixedNow).Detail(context.Background(), request(report.Failure, ev)); err != nil {
		t.Fatal(err)
	}
	total, first := 0, true
	for _, c := range f.snapshot() {
		if c.Path != pathUpdate {
			continue
		}
		var b checkBody
		_ = json.Unmarshal(c.Body, &b)
		total += len(b.Output.Annotations)
		if first && (len(b.Output.Annotations) != 50 || b.Output.Annotations[0].Start != 4) {
			t.Fatalf("the first page must resume after the 3 held annotations: %+v", b.Output.Annotations[:1])
		}
		first = false
		if b.Output.Text == "" {
			t.Fatal("every page carries the table text")
		}
	}
	if total != 117 {
		t.Fatalf("sent %d, want 117", total)
	}
}

// TestMarkdownInjectionEscaped: check text from a repository cannot make links, images, HTML or mentions.
func TestMarkdownInjectionEscaped(t *testing.T) {
	ev := fixtureEvidence(check("x", "fail", func(c *model.Check) {
		c.ID = "[evil](https://attacker.example)![i](https://attacker.example/p.png)<b>@owner"
		c.Remediation = "<img src=x> [a](b)"
	}))
	out := Table(ev, model.Projection(ev))
	// the id is shown in a code span (rendered literally); every other cell is escaped
	if !strings.Contains(out, "| `[evil](https://attacker.example)") || !strings.Contains(out, `\<img src=x\> \[a\](b)`) {
		t.Fatalf("not escaped:\n%s", out)
	}
	sum := cell("allowed failures: [x](https://attacker.example) @owner")
	if strings.Contains(sum, "[x]") || strings.Contains(sum, " @owner") {
		t.Fatalf("summary text not escaped: %s", sum)
	}
}

// TestTokenPerRepository: a token is pinned to and cached per repository.
func TestTokenPerRepository(t *testing.T) {
	key, _ := testKey(t)
	f := newFake(t, key)
	p := newPublisher(t, f, fixedNow)
	for _, repo := range []string{"widgets", "gadgets", "widgets"} {
		if _, err := p.tokens(repo).Token(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count("POST", "/app/installations/{id}/access_tokens"); n != 2 {
		t.Fatalf("exchanges %d, want 2 (one per repository)", n)
	}
}

// TestFailedExchangeIsHeld: a repository the installation does not cover costs one exchange per window, not one per run.
func TestFailedExchangeIsHeld(t *testing.T) {
	key, _ := testKey(t)
	f := newFake(t, key)
	f.failPath = "/app/installations/{id}/access_tokens"
	f.failErr = errBoom
	clock := t0
	p := newPublisher(t, f, func() time.Time { return clock })
	ts := p.tokens("outside")
	for i := 0; i < 3; i++ {
		if _, err := ts.Token(context.Background()); err == nil {
			t.Fatal("want an error")
		}
	}
	if n := f.count("POST", "/app/installations/{id}/access_tokens"); n != 1 {
		t.Fatalf("exchanges %d, want 1 inside the hold window", n)
	}
	clock = t0.Add(failureHold)
	_, _ = ts.Token(context.Background())
	if n := f.count("POST", "/app/installations/{id}/access_tokens"); n != 2 {
		t.Fatalf("exchanges %d, want a retry after the window", n)
	}
}

func TestEmptyMessageAnnotationSkipped(t *testing.T) {
	ev := fixtureEvidence(check("a", "fail", func(c *model.Check) { c.Excerpt = "x/y.go:10: \x01\x02\nx/y.go:11: real" }))
	if got := Annotations(ev); len(got) != 1 || got[0].Start != 11 {
		t.Fatalf("%+v", got)
	}
	opt := fixtureEvidence(check("a", "fail", func(c *model.Check) { c.Required = false; c.Excerpt = "x/y.go:1: optional" }))
	if got := Annotations(opt); len(got) != 1 || got[0].Level != "warning" {
		t.Fatalf("an optional check must annotate as a warning: %+v", got)
	}
}
