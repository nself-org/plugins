package protect

// Purpose: the main-status auditor of `policy protect --audit <days>` and the
// `main-status-audit` drill (P7-CI-07): list default-branch commits of the
// last N days that have no green `nself-ci` status and no green `nself-ci`
// check run.
// Inputs:  a Client, the repository, the branch, the window in days.
// Outputs: an AuditReport; Missing is empty when every commit is green.
// Constraints: read only (gh api GETs). A commit is green when its combined
// status lists a `nself-ci` status in state success, or a check run named
// `nself-ci` completed with conclusion success (D18: statuses survive only on
// same-repo runs, check runs on fork and Dependabot runs). At most
// MaxCommits commits are looked at; Truncated says so.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// MaxCommits bounds one audit (two API calls per commit).
const MaxCommits = 200

// Commit is one audited commit.
type Commit struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
}

// AuditReport is the auditor's answer.
type AuditReport struct {
	Repo      string   `json:"repo"`
	Branch    string   `json:"branch"`
	Days      int      `json:"days"`
	Checked   int      `json:"checked"`
	Truncated bool     `json:"truncated"`
	Missing   []Commit `json:"missing"`
}

// Audit checks the commits of branch from the last days days. A check run
// counts only when the app that created it is appID (0: the default, AppID),
// the same pin the required check carries; a status has no app to compare.
func (c Client) Audit(ctx context.Context, repo, branch string, days int, now time.Time, appID int) (AuditReport, error) {
	if appID == 0 {
		appID = AppID
	}
	rep := AuditReport{Repo: repo, Branch: branch, Days: days, Missing: []Commit{}}
	since := now.UTC().Add(-time.Duration(days) * 24 * time.Hour).Format(time.RFC3339)
	out, err := c.call(ctx, "--method", "GET", "-f", "sha="+branch, "-f", "since="+since, "-f", "per_page=100", "--paginate",
		"--jq", `.[] | [.sha, (.commit.message | split("\n")[0])] | @tsv`, "repos/"+repo+"/commits")
	if err != nil {
		return rep, err
	}
	var commits []Commit
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		sha, subject, _ := strings.Cut(line, "\t")
		commits = append(commits, Commit{SHA: sha, Subject: subject})
	}
	if len(commits) > MaxCommits {
		commits, rep.Truncated = commits[:MaxCommits], true
	}
	for _, cm := range commits {
		green, err := c.green(ctx, repo, cm.SHA, appID)
		if err != nil {
			return rep, err
		}
		rep.Checked++
		if !green {
			rep.Missing = append(rep.Missing, cm)
		}
	}
	return rep, nil
}

type statusDoc struct {
	Statuses []struct {
		Context string `json:"context"`
		State   string `json:"state"`
	} `json:"statuses"`
}

type checkRunsDoc struct {
	CheckRuns []struct {
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		App        struct {
			ID int `json:"id"`
		} `json:"app"`
	} `json:"check_runs"`
}

// green reports whether sha carries a green nself-ci status or check run.
func (c Client) green(ctx context.Context, repo, sha string, appID int) (bool, error) {
	out, err := c.call(ctx, "repos/"+repo+"/commits/"+sha+"/status")
	if err != nil {
		return false, err
	}
	var st statusDoc
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		return false, fmt.Errorf("protect: the status answer for %.12s is not JSON: %w", sha, err)
	}
	for _, s := range st.Statuses {
		if s.Context == Context && s.State == "success" {
			return true, nil
		}
	}
	out, err = c.call(ctx, "repos/"+repo+"/commits/"+sha+"/check-runs?check_name="+Context)
	if err != nil {
		return false, err
	}
	var cr checkRunsDoc
	if err := json.Unmarshal([]byte(out), &cr); err != nil {
		return false, fmt.Errorf("protect: the check-runs answer for %.12s is not JSON: %w", sha, err)
	}
	for _, r := range cr.CheckRuns {
		if r.Name == Context && r.App.ID == appID && r.Status == "completed" && r.Conclusion == "success" {
			return true, nil
		}
	}
	return false, nil
}
