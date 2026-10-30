package report

import (
	"context"
	"strings"
	"testing"
)

const sha40 = "0123456789abcdef0123456789abcdef01234567"

func ghFake(out string, exit int, got *[]string) GH {
	return GH{Capture: func(_ context.Context, argv ...string) (string, int, error) {
		*got = argv
		return out, exit, nil
	}}
}

func TestLedgerGHListStatuses(t *testing.T) {
	var argv []string
	body := "HTTP/2.0 200 OK\r\nDate: Tue, 27 Oct 2026 12:00:00 GMT\r\n\r\n" +
		`[{"context":"nself-ci","state":"success","description":"ok","created_at":"2026-10-27T11:00:00Z","creator":{"login":"station"}}]`
	l, now, err := ghFake(body, 0, &argv).ListStatuses(context.Background(), "o", "r", sha40)
	if err != nil || len(l) != 1 || l[0].Creator != "station" || l[0].State != Success {
		t.Fatalf("%v %v", l, err)
	}
	if now.Year() != 2026 || now.Hour() != 12 {
		t.Fatalf("provider time %v", now)
	}
	if !strings.Contains(strings.Join(argv, " "), "commits/"+sha40+"/statuses") {
		t.Fatalf("%v", argv)
	}
	if _, _, err = ghFake("boom", 1, &argv).ListStatuses(context.Background(), "o", "r", sha40); err == nil {
		t.Fatal("exit 1 must error")
	}
	if _, _, err = ghFake("x", 0, &argv).ListStatuses(context.Background(), "o", "r", "zz"); err == nil {
		t.Fatal("bad sha")
	}
}

func TestLedgerGHPostClaim(t *testing.T) {
	var argv []string
	g := ghFake("HTTP/2.0 201 Created\nDate: Tue, 27 Oct 2026 12:00:01 GMT\n\n{}", 0, &argv)
	now, err := g.PostClaim(context.Background(), ClaimStatus("o", "r", sha40, "mm6", "i1"))
	if err != nil || now.Second() != 1 {
		t.Fatalf("%v %v", now, err)
	}
	j := strings.Join(argv, " ")
	if !strings.Contains(j, "-i") || !strings.Contains(j, "context="+ClaimContext) || !strings.Contains(j, "description=claim mm6 i1") {
		t.Fatalf("%s", j)
	}
	res := Status{Owner: "o", Repo: "r", Revision: sha40, State: Success}
	if _, err = g.PostClaim(context.Background(), res); err == nil {
		t.Fatal("a result is not a claim")
	}
	if _, err = ghFake("down", 1, &argv).PostClaim(context.Background(), ClaimStatus("o", "r", sha40, "mm6", "i1")); err == nil {
		t.Fatal("provider failure must surface")
	}
}
