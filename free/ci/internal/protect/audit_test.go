package protect

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func greenStatus(ctx string) string {
	return fmt.Sprintf(`{"state":"success","statuses":[{"context":%q,"state":"success"}]}`, ctx)
}

const noStatuses = `{"state":"pending","statuses":[]}`
const noRuns = `{"total_count":0,"check_runs":[]}`

func runs(name, status, conclusion string) string { return runsApp(name, status, conclusion, AppID) }

func runsApp(name, status, conclusion string, app int) string {
	return fmt.Sprintf(`{"total_count":1,"check_runs":[{"name":%q,"status":%q,"conclusion":%q,"app":{"id":%d}}]}`, name, status, conclusion, app)
}

// TestAuditListsCommitsWithoutGreen: green status, green check run, red status,
// pending check run and a green check of another name.
func TestAuditListsCommitsWithoutGreen(t *testing.T) {
	list := strings.Join([]string{
		"aaaa\tfeat: green status",
		"bbbb\tfix: green check run",
		"cccc\tchore: red status",
		"dddd\tchore: pending check run",
		"eeee\tdocs: other check only",
	}, "\n")
	f := &fakeGH{replies: []reply{
		{match: "--paginate", out: list},
		{match: "commits/aaaa/status", out: greenStatus("nself-ci")},
		{match: "commits/bbbb/status", out: noStatuses},
		{match: "commits/bbbb/check-runs", out: runs("nself-ci", "completed", "success")},
		{match: "commits/cccc/status", out: `{"statuses":[{"context":"nself-ci","state":"failure"}]}`},
		{match: "commits/cccc/check-runs", out: noRuns},
		{match: "commits/dddd/status", out: noStatuses},
		{match: "commits/dddd/check-runs", out: runs("nself-ci", "in_progress", "")},
		{match: "commits/eeee/status", out: greenStatus("lint")},
		{match: "commits/eeee/check-runs", out: runs("lint", "completed", "success")},
	}}
	rep, err := f.client().Audit(context.Background(), "example-org/example-repo", "main", 7, t0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Checked != 5 || rep.Truncated || rep.Days != 7 {
		t.Fatalf("report = %+v", rep)
	}
	var shas []string
	for _, m := range rep.Missing {
		shas = append(shas, m.SHA)
	}
	if strings.Join(shas, ",") != "cccc,dddd,eeee" || rep.Missing[0].Subject != "chore: red status" {
		t.Fatalf("missing = %+v", rep.Missing)
	}
	if first := f.calls[0]; !strings.Contains(first, "since=2026-10-12T12:00:00Z") || !strings.Contains(first, "sha=main") {
		t.Fatalf("list call = %s", first)
	}
}

func TestAuditAllGreenIsEmpty(t *testing.T) {
	f := &fakeGH{replies: []reply{
		{match: "--paginate", out: "aaaa\tone\n"},
		{match: "commits/aaaa/status", out: greenStatus("nself-ci")},
	}}
	rep, err := f.client().Audit(context.Background(), "example-org/example-repo", "main", 3, t0, 0)
	if err != nil || len(rep.Missing) != 0 || rep.Checked != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	if rep.Missing == nil {
		t.Fatal("Missing must marshal as [], not null")
	}
}

func TestAuditTruncatesAndErrors(t *testing.T) {
	var lines []string
	for i := 0; i < MaxCommits+5; i++ {
		lines = append(lines, fmt.Sprintf("%040d\tc%d", i, i))
	}
	f := &fakeGH{replies: []reply{
		{match: "--paginate", out: strings.Join(lines, "\n")},
		{match: "/status", out: greenStatus("nself-ci")},
	}}
	rep, err := f.client().Audit(context.Background(), "example-org/example-repo", "main", 30, t0, 0)
	if err != nil || !rep.Truncated || rep.Checked != MaxCommits {
		t.Fatalf("%+v %v", rep, err)
	}
	bad := &fakeGH{replies: []reply{{match: "--paginate", exit: 1, out: "gh: Not Found (HTTP 404)"}}}
	if _, err := bad.client().Audit(context.Background(), "example-org/example-repo", "main", 1, t0, 0); err == nil {
		t.Fatal("a failed commit listing must fail the audit, not report green")
	}
	junk := &fakeGH{replies: []reply{{match: "--paginate", out: "aaaa\tone"}, {match: "/status", out: "<html>"}}}
	if _, err := junk.client().Audit(context.Background(), "example-org/example-repo", "main", 1, t0, 0); err == nil {
		t.Fatal("an unreadable status answer must fail the audit")
	}
}

// TestAuditCheckRunMustComeFromThePinnedApp: a success check run named nself-ci
// from another app does not make the commit green.
func TestAuditCheckRunMustComeFromThePinnedApp(t *testing.T) {
	f := &fakeGH{replies: []reply{
		{match: "--paginate", out: "aaaa\tone"},
		{match: "commits/aaaa/status", out: noStatuses},
		{match: "commits/aaaa/check-runs", out: runsApp("nself-ci", "completed", "success", 4242)},
	}}
	rep, err := f.client().Audit(context.Background(), "example-org/example-repo", "main", 1, t0, 0)
	if err != nil || len(rep.Missing) != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	f2 := &fakeGH{replies: []reply{
		{match: "--paginate", out: "aaaa\tone"},
		{match: "commits/aaaa/status", out: noStatuses},
		{match: "commits/aaaa/check-runs", out: runsApp("nself-ci", "completed", "success", 4242)},
	}}
	rep, err = f2.client().Audit(context.Background(), "example-org/example-repo", "main", 1, t0, 4242)
	if err != nil || len(rep.Missing) != 0 {
		t.Fatalf("an overridden app id must be honoured: %+v %v", rep, err)
	}
}
