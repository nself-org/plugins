package drill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func fixtureDoc(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "repo", "scripts", "drill", name, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestJudge(t *testing.T) {
	pass, failing, invalid := fixtureDoc(t, "pass"), fixtureDoc(t, "fail-nested"), fixtureDoc(t, "invalid")
	for _, tc := range []struct {
		name      string
		out       string
		truncated bool
		known     bool
		fail      bool
		class     string
		code      string
		findings  int
	}{
		{"no document keeps the exit code", "drill starting\nall good\n", false, false, false, "", "", 0},
		{"a passing document", "noise\n" + pass, false, true, false, "", "", 0},
		{"a failing document ends failed", "noise\n" + failing, false, true, true, "code", "", 1},
		{"an invalid document ends error", invalid, false, true, true, "code", CodeInvalid, 0},
		{"the last document wins", failing + "later noise\n" + pass, false, true, false, "", "", 0},
		{"output after the document is fine", pass + "bye\n", false, true, false, "", "", 0},
		{"a cut first line is ignored", "cut off {\"schema\":\"ci.evidence/v1\"\n" + pass, true, true, false, "", "", 0},
		{"a cut document is not trusted", "{\"schema\":\"ci.evidence/v1\",\"revi", true, false, false, "", "", 0},
	} {
		ch, known := Judge(tc.out, tc.truncated)
		if known != tc.known || ch.Fail != tc.fail || string(ch.FailureClass) != tc.class || ch.Findings != tc.findings {
			t.Errorf("%s: known=%v %+v", tc.name, known, ch)
			continue
		}
		if tc.code != "" && cirun.CodeOf(ch.Err) != tc.code {
			t.Errorf("%s: code %q, want %q", tc.name, cirun.CodeOf(ch.Err), tc.code)
		}
	}
	ch, _ := Judge(failing, false)
	if !strings.Contains(ch.Excerpt, "step-two") || !strings.Contains(ch.Excerpt, "assertion failed") {
		t.Errorf("excerpt does not name the failing nested check: %q", ch.Excerpt)
	}
}

// TestDrillNested runs the fixture pipeline through the real coordinator: one
// check per drill directory, the exit code is the result, and a nested
// document that fails or is invalid never ends passed.
func TestDrillNested(t *testing.T) {
	dir := fixtureRepo(t)
	p, _ := planDrills(t, dir)
	res := runDrills(t, dir, p)
	ev := res.Summary.Doc
	if ev.Pipeline.Kind != "drill" || len(ev.Checks) != 3 || ev.Counts.Total != 3 {
		t.Fatalf("pipeline %+v with %d checks", ev.Pipeline, len(ev.Checks))
	}
	checks := checksByID(ev)
	if c := checks["drill-pass"]; c.Result != "pass" || c.Findings != 0 {
		t.Errorf("passing drill: %+v", c)
	}
	nested := checks["drill-fail-nested"]
	if nested.Result != "fail" || nested.Findings != 1 || !strings.Contains(nested.Excerpt, "step-two") || nested.FailureClass != "code" {
		t.Errorf("exit 0 with a failing nested result must end fail: %+v", nested)
	}
	bad := checks["drill-invalid"]
	if bad.Result != "error" || !strings.Contains(bad.Excerpt, CodeInvalid) {
		t.Errorf("invalid evidence must end error with E640: %+v", bad)
	}
	if ev.Result != "fail" && ev.Result != "error" || res.Exit == 0 {
		t.Errorf("a pipeline with a failing drill must not pass: result %s exit %d", ev.Result, res.Exit)
	}
}

func TestDrillExitCodeIsTheResult(t *testing.T) {
	repo := fixtureRepo(t)
	for name, script := range map[string]string{
		"ok":     "#!/bin/sh\nexit 0\n",
		"broken": "#!/bin/sh\necho oops >&2\nexit 3\n",
		"noisy":  "#!/bin/sh\necho '{\"not\":\"evidence\"}'\nexit 0\n",
	} {
		addDrill(t, repo, name, script)
	}
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-q", "-m", "more drills")
	p, _ := planDrills(t, repo)
	checks := checksByID(runDrills(t, repo, p).Summary.Doc)
	if len(checks) != 6 {
		t.Fatalf("adding drill directories must add jobs with no config edit: %d checks", len(checks))
	}
	for id, want := range map[string]string{"drill-ok": "pass", "drill-broken": "fail", "drill-noisy": "pass"} {
		if got := checks[id].Result; string(got) != want {
			t.Errorf("%s = %s, want %s", id, got, want)
		}
	}
}
