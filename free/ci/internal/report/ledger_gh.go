package report

// Purpose: the GitHub implementation of Ledger: one synchronous POST for a
// claim and one GET of /commits/{sha}/statuses for the history, both read
// with response headers so the provider's Date is the clock.
// Inputs:  gh api through the capture funnel. Outputs: statuses and a time.
// Constraints: gh authenticates itself; one list call per sha per pass.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var _ Ledger = GH{}

// PostClaim implements Ledger.
func (g GH) PostClaim(ctx context.Context, s Status) (time.Time, error) {
	if err := s.Validate(); err != nil {
		return time.Time{}, err
	}
	if s.ContextName() != ClaimContext || s.State != Pending {
		return time.Time{}, errors.New("report: a claim is a pending status on the claim context")
	}
	argv := Args(s)
	argv = append(argv[:2], append([]string{"-i"}, argv[2:]...)...)
	out, exit, err := g.capture(ctx, argv...)
	if err != nil {
		return time.Time{}, fmt.Errorf("report: gh not run: %w", err)
	}
	if exit != 0 {
		return time.Time{}, fmt.Errorf("report: claim not posted (exit %d): %s", exit, firstLine(out))
	}
	now, _ := splitResponse(out)
	return now, nil
}

// ListStatuses implements Ledger.
func (g GH) ListStatuses(ctx context.Context, owner, repo, sha string) ([]LedgerStatus, time.Time, error) {
	if err := (Status{Owner: owner, Repo: repo, Revision: sha, State: Pending}).Validate(); err != nil {
		return nil, time.Time{}, err
	}
	path := fmt.Sprintf("repos/%s/%s/commits/%s/statuses?per_page=100", owner, repo, sha)
	out, exit, err := g.capture(ctx, "gh", "api", "-i", "--paginate", path)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("report: gh not run: %w", err)
	}
	if exit != 0 {
		return nil, time.Time{}, fmt.Errorf("report: statuses not read (exit %d): %s", exit, firstLine(out))
	}
	now, body := splitResponse(out)
	list, err := parseStatuses(body)
	return list, now, err
}

// splitResponse separates `gh api -i` headers from the body and reads Date.
func splitResponse(out string) (time.Time, string) {
	out = strings.ReplaceAll(out, "\r\n", "\n")
	head, body, ok := strings.Cut(out, "\n\n")
	if !ok || !strings.HasPrefix(head, "HTTP/") {
		return time.Time{}, out
	}
	var date time.Time
	for _, l := range strings.Split(head, "\n") {
		if k, v, found := strings.Cut(l, ":"); found && strings.EqualFold(k, "date") {
			date, _ = http.ParseTime(strings.TrimSpace(v))
		}
	}
	return date, body
}

func parseStatuses(body string) ([]LedgerStatus, error) {
	var raw []struct {
		Context     string    `json:"context"`
		State       State     `json:"state"`
		Description string    `json:"description"`
		CreatedAt   time.Time `json:"created_at"`
		Creator     struct {
			Login string `json:"login"`
		} `json:"creator"`
	}
	body = strings.ReplaceAll(strings.TrimSpace(body), "\n][", ",") // paginated pages
	body = strings.ReplaceAll(body, "][", ",")
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return nil, fmt.Errorf("report: statuses unreadable: %w", err)
	}
	out := make([]LedgerStatus, 0, len(raw))
	for _, r := range raw {
		out = append(out, LedgerStatus{Context: r.Context, State: r.State, Creator: r.Creator.Login,
			Description: r.Description, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// Login returns the authenticated login of the station token, the creator
// identity whose statuses count.
func (g GH) Login(ctx context.Context) (string, error) {
	out, exit, err := g.capture(ctx, "gh", "api", "user", "-q", ".login")
	if err != nil || exit != 0 || strings.TrimSpace(out) == "" {
		return "", errors.New("report: station login unreadable")
	}
	return strings.TrimSpace(out), nil
}
