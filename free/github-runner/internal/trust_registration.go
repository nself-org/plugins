package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// runnerRef identifies a runner: by id when GitHub issued one (the persisted
// registration), else by display name.
type runnerRef struct {
	ID   int64
	Name string
}

func (r runnerRef) matches(o runnerRef) bool {
	if r.ID != 0 {
		return o.ID == r.ID
	}
	return r.Name != "" && strings.EqualFold(o.Name, r.Name)
}

func (r runnerRef) String() string {
	if r.Name != "" {
		return strconv.Quote(r.Name)
	}
	return "#" + strconv.FormatInt(r.ID, 10)
}

// persistedRunner reads the registration the runner binary wrote to
// RunnerDir/.runner. Absent means not registered (nil, nil). A file that
// exists but cannot be read or parsed is an error: the guard fails closed
// rather than trust the configured name alone.
func persistedRunner(dir string) (*runnerRef, error) {
	if dir == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".runner"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Join(dir, ".runner"), err)
	}
	var f struct {
		AgentID   json.Number `json:"agentId"`
		AgentName string      `json:"agentName"`
	}
	// The runner writes the file with a UTF-8 BOM.
	if err := json.Unmarshal(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")), &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Join(dir, ".runner"), err)
	}
	ref := runnerRef{Name: f.AgentName}
	if f.AgentID != "" {
		if ref.ID, err = f.AgentID.Int64(); err != nil {
			return nil, fmt.Errorf("parse agentId in %s: %w", filepath.Join(dir, ".runner"), err)
		}
	}
	if ref.ID == 0 && ref.Name == "" {
		return nil, fmt.Errorf("%s names no runner", filepath.Join(dir, ".runner"))
	}
	return &ref, nil
}

// verifyRegisteredGroup refuses a runner that GitHub has registered outside
// the configured group: the guard reads the configured group, so a runner left
// in another (possibly public) group by a config change would otherwise pass.
// Both the configured name and the registration persisted in the runner dir
// are checked, so renaming the runner (GITHUB_RUNNER_NAME or the hostname) does
// not hide the installed one. A runner that is not registered at all passes (it
// is about to be).
func verifyRegisteredGroup(ctx context.Context, cfg TrustConfig, g *runnerGroup) error {
	var refs []runnerRef
	if cfg.RunnerName != "" {
		refs = append(refs, runnerRef{Name: cfg.RunnerName})
	}
	persisted, err := persistedRunner(cfg.RunnerDir)
	if err != nil {
		return &TrustError{Code: CodeTrustUnverifiable, Status: http.StatusBadGateway,
			Message: fmt.Sprintf("the installed runner registration cannot be read, so its runner group cannot be verified: %v", err)}
	}
	if persisted != nil {
		refs = append(refs, *persisted)
	}
	if len(refs) == 0 {
		return nil
	}
	if g.ID == 0 {
		return &TrustError{Code: CodeTrustUnverifiable, Status: http.StatusBadGateway,
			Message: fmt.Sprintf("GitHub did not report the id of runner group %q", g.Name)}
	}
	org := url.PathEscape(cfg.Org)
	inGroup, err := listRunners(ctx, cfg, fmt.Sprintf("%s/orgs/%s/actions/runner-groups/%d/runners", githubAPIBase, org, g.ID))
	if err != nil {
		return err
	}
	var inOrg []runnerRef
	orgLoaded := false
	for _, ref := range refs {
		if anyMatch(ref, inGroup) {
			continue
		}
		if !orgLoaded {
			if inOrg, err = listRunners(ctx, cfg, fmt.Sprintf("%s/orgs/%s/actions/runners", githubAPIBase, org)); err != nil {
				return err
			}
			orgLoaded = true
		}
		if !anyMatch(ref, inOrg) {
			continue
		}
		return &TrustError{Code: CodeRunnerGroupMismatch, Status: http.StatusConflict,
			Message: fmt.Sprintf("runner %s is registered in a different runner group from the configured %q, so the check above does not cover it. "+
				"Re-register the runner (install again) so it joins %q, or set GITHUB_RUNNER_GROUP to the group it is in", ref, g.Name, g.Name)}
	}
	return nil
}

func anyMatch(ref runnerRef, list []runnerRef) bool {
	for _, r := range list {
		if ref.matches(r) {
			return true
		}
	}
	return false
}

// listRunners pages through a runner list.
func listRunners(ctx context.Context, cfg TrustConfig, endpoint string) ([]runnerRef, error) {
	var all []runnerRef
	for page := 1; page <= maxGroupPages; page++ {
		var body struct {
			Runners []struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			} `json:"runners"`
		}
		if err := githubGet(ctx, cfg.PAT, fmt.Sprintf("%s?per_page=%d&page=%d", endpoint, groupsPageSize, page), &body); err != nil {
			return nil, err
		}
		for _, r := range body.Runners {
			all = append(all, runnerRef{ID: r.ID, Name: r.Name})
		}
		if len(body.Runners) < groupsPageSize {
			return all, nil
		}
	}
	return nil, &TrustError{Code: CodeTrustUnverifiable, Status: http.StatusBadGateway,
		Message: "the runner list is too long to search"}
}
