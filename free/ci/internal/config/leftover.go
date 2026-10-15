package config

// Purpose: find legacy `.ci.yaml` manifests and refuse them under V15
// (E621, Epic D7).
// Inputs:  a repo root.
// Outputs: the repo-relative paths of every `.ci.yaml` (Discover) or an
// *Error (E621) naming each one and the migration command (CheckLeftover).
// Constraints: the search depth matches the legacy runner (3 levels below the
// root) so a file the old engine would run is never missed; `.git` is not
// searched. The orchestrator calls CheckLeftover before planning a run
// (P7-CI-37); `ci config migrate` does not call it, since migration is how a
// repo gets out of the state.

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
)

// ErrNothingToMigrate is returned by Migrate when the repo has no `.ci.yaml`.
var ErrNothingToMigrate = errors.New("no .ci.yaml found to migrate")

const (
	legacyName     = ".ci.yaml"
	legacyMaxDepth = 3
)

// Discover lists every `.ci.yaml` under root, sorted, repo-relative.
func Discover(root string) ([]string, error) {
	var found []string
	var walk func(dir string, depth int) error
	walk = func(dir string, depth int) error {
		if depth > legacyMaxDepth {
			return nil
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if depth == 0 {
				return err
			}
			return nil
		}
		for _, e := range entries {
			switch {
			case e.IsDir() && e.Name() != ".git":
				if err := walk(filepath.Join(dir, e.Name()), depth+1); err != nil {
					return err
				}
			case e.Name() == legacyName && !e.IsDir():
				rel, err := filepath.Rel(root, filepath.Join(dir, e.Name()))
				if err != nil {
					return err
				}
				found = append(found, filepath.ToSlash(rel))
			}
		}
		return nil
	}
	if err := walk(root, 0); err != nil {
		return nil, err
	}
	sort.Strings(found)
	return found, nil
}

// CheckLeftover returns an E621 error when root holds any `.ci.yaml`.
func CheckLeftover(root string) error {
	found, err := Discover(root)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return nil
	}
	problems := make([]Problem, len(found))
	for i, f := range found {
		problems[i] = Problem{File: f, Message: "legacy manifest under V15; run `nself ci config migrate --write`, then delete this file"}
	}
	return &Error{Code: CodeLeftover, Problems: problems}
}
