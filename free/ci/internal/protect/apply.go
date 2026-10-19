package protect

// Purpose: apply a Plan: save the previous protection, PUT the new body, read
// it back and compare (P7-CI-07, the snapshot/restore pattern of Constitution
// 14.3). Restore puts a saved snapshot back.
// Inputs:  a Client, a Plan, the snapshot directory ($NSELF_CI_HOME/protect).
// Outputs: the snapshot path and the verified result, or an error.
// Constraints: the snapshot is written before the PUT and the PUT is skipped
// when the snapshot cannot be written; a PUT whose read-back differs from the
// body is an error that names the differing fields. Changing live protection
// is owner-named (P7-CI-55): the command calls this only without --dry-run.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Snapshot is the file written before a PUT.
type Snapshot struct {
	Repo        string          `json:"repo"`
	Branch      string          `json:"branch"`
	TakenAt     string          `json:"taken_at"`
	Unprotected bool            `json:"unprotected"`
	Protection  json.RawMessage `json:"protection,omitempty"`
}

func snapshotName(repo, branch string, at time.Time) string {
	r := strings.NewReplacer("/", "__", " ", "_")
	return fmt.Sprintf("%s__%s-%s.json", r.Replace(repo), r.Replace(branch), at.UTC().Format("20060102T150405Z"))
}

// SaveSnapshot writes the live answer (nil: unprotected) under dir and
// returns the path. The directory and file are private to the user.
func SaveSnapshot(dir, repo, branch string, live []byte, at time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("protect: cannot create the snapshot directory: %w", err)
	}
	s := Snapshot{Repo: repo, Branch: branch, TakenAt: at.UTC().Format(time.RFC3339), Unprotected: live == nil}
	if live != nil {
		s.Protection = json.RawMessage(live)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", err
	}
	// O_EXCL: two snapshots of one branch in the same second get different names;
	// an earlier recovery point is never overwritten.
	base := strings.TrimSuffix(snapshotName(repo, branch, at), ".json")
	for n := 0; n < 100; n++ {
		name := base + ".json"
		if n > 0 {
			name = fmt.Sprintf("%s-%d.json", base, n)
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("protect: cannot write the snapshot: %w", err)
		}
		_, werr := f.Write(append(data, '\n'))
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return "", fmt.Errorf("protect: cannot write the snapshot: %w", werr)
		}
		return path, nil
	}
	return "", fmt.Errorf("protect: too many snapshots of %s %s in one second", repo, branch)
}

// LoadSnapshot reads a snapshot file.
func LoadSnapshot(path string) (Snapshot, error) {
	var s Snapshot
	data, err := os.ReadFile(path)
	if err != nil {
		return s, fmt.Errorf("protect: cannot read the snapshot: %w", err)
	}
	if err := json.Unmarshal(data, &s); err != nil || s.Repo == "" || s.Branch == "" {
		return s, fmt.Errorf("protect: %s is not a protection snapshot", path)
	}
	return s, nil
}

// putAndVerify writes body and reads it back.
func (c Client) putAndVerify(ctx context.Context, repo, branch string, body Body, tmpDir string) error {
	f, err := os.CreateTemp(tmpDir, "put-*.json")
	if err != nil {
		return fmt.Errorf("protect: cannot stage the PUT body: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(mustIndent(body)); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := c.Put(ctx, repo, branch, f.Name()); err != nil {
		return err
	}
	after, err := c.Protection(ctx, repo, branch)
	if err != nil {
		return fmt.Errorf("protect: the PUT was accepted but the read-back failed: %w", err)
	}
	if after == nil {
		return fmt.Errorf("protect: the PUT was accepted but %s/%s reads as unprotected", repo, branch)
	}
	got, err := Normalise(after)
	if err != nil {
		return err
	}
	if diff := Diff(body, got); len(diff) > 0 {
		return fmt.Errorf("protect: the read-back differs from the PUT body: %s", strings.Join(diff, "; "))
	}
	return nil
}

// Apply saves the previous protection under snapDir, then PUTs the plan's
// body and verifies it. It returns the snapshot path. A no-op plan changes
// nothing and writes nothing.
func (c Client) Apply(ctx context.Context, p Plan, live []byte, snapDir string, now time.Time) (string, error) {
	if p.Noop() {
		return "", nil
	}
	path, err := SaveSnapshot(snapDir, p.Repo, p.Branch, live, now)
	if err != nil {
		return "", err
	}
	return path, c.putAndVerify(ctx, p.Repo, p.Branch, p.Body, snapDir)
}

// Restore puts a snapshot back and returns the path of a new snapshot of what
// was there before (so a restore can itself be undone). A snapshot of an
// unprotected branch cannot be restored by PUT: that needs DELETE, which this
// tool never does.
func (c Client) Restore(ctx context.Context, snap Snapshot, snapDir string, now time.Time) (string, error) {
	if snap.Unprotected || len(snap.Protection) == 0 {
		return "", fmt.Errorf("protect: the snapshot is of an unprotected branch; removing protection is not something this command does")
	}
	body, err := Normalise(snap.Protection)
	if err != nil {
		return "", err
	}
	live, err := c.Protection(ctx, snap.Repo, snap.Branch)
	if err != nil {
		return "", err
	}
	path, err := SaveSnapshot(snapDir, snap.Repo, snap.Branch, live, now)
	if err != nil {
		return "", err
	}
	return path, c.putAndVerify(ctx, snap.Repo, snap.Branch, body, snapDir)
}

// Diff names the top-level fields on which a and b differ.
func Diff(a, b Body) []string {
	ja, jb := toMap(a), toMap(b)
	var out []string
	for _, k := range sortedKeys(ja, jb) {
		if string(ja[k]) != string(jb[k]) {
			out = append(out, fmt.Sprintf("%s: expected %s, got %s", k, ja[k], jb[k]))
		}
	}
	return out
}

// toMap marshals b with its checks sorted: GitHub may list them in another
// order than the PUT sent, which is not a difference.
func toMap(b Body) map[string]json.RawMessage {
	if b.RequiredStatusChecks != nil {
		sc := *b.RequiredStatusChecks
		sc.Checks = append([]Check(nil), sc.Checks...)
		sort.Slice(sc.Checks, func(i, j int) bool {
			if sc.Checks[i].Context != sc.Checks[j].Context {
				return sc.Checks[i].Context < sc.Checks[j].Context
			}
			return sc.Checks[i].AppID < sc.Checks[j].AppID
		})
		b.RequiredStatusChecks = &sc
	}
	data, _ := json.Marshal(b)
	m := map[string]json.RawMessage{}
	_ = json.Unmarshal(data, &m)
	return m
}

func sortedKeys(ms ...map[string]json.RawMessage) []string {
	seen := map[string]bool{}
	var ks []string
	for _, m := range ms {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				ks = append(ks, k)
			}
		}
	}
	sort.Strings(ks)
	return ks
}
