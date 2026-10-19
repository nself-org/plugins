package reproduce

// Purpose: the durable link between a reproduction and the run it
// reproduces. One JSON line per reproduction, kept per original run under
// <state>/reproductions/<original run id>.jsonl.
// Inputs:  a state directory ("" = NSELF_CI_HOME, else ~/.nself/ci) and a Link.
// Outputs: appended lines; Links reads them back oldest first.
// Constraints: the file name is the validated run id (no path escape); the
// directory is 0700 and the file 0600; a link names ids and classes only, no
// output and no environment.

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/logs"
)

// LinkSchema names the line format.
const LinkSchema = "ci.reproduction/v1"

// Link records that Reproduction re-ran the job of Original.
type Link struct {
	Schema       string  `json:"schema"`
	At           string  `json:"at"`
	Job          string  `json:"job"`
	Revision     string  `json:"revision"`
	Original     Outcome `json:"original"`
	Reproduction Outcome `json:"reproduction"`
	// Reproduced is true when the re-run ended with the same result and the same failure class.
	Reproduced bool     `json:"reproduced"`
	Drift      []Drift  `json:"drift"`
	Warnings   []string `json:"warnings"`
}

func linkPath(home, runID string) (string, error) {
	state := logs.Home(home)
	if state == "" || !logs.ValidID(runID) {
		return "", fail(evidence.CodeWorktree, "no usable state directory or run id for the reproduction link")
	}
	return filepath.Join(state, "reproductions", runID+".jsonl"), nil
}

// SaveLink appends l to the file of its original run.
func SaveLink(home string, l Link) error {
	p, err := linkPath(home, l.Original.RunID)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fail(evidence.CodeWorktree, "cannot create %s: %v", filepath.Dir(p), err)
	}
	line, err := json.Marshal(l)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fail(evidence.CodeWorktree, "cannot write %s: %v", p, err)
	}
	if _, err = f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return fail(evidence.CodeWorktree, "cannot write %s: %v", p, err)
	}
	return f.Close()
}

// Links returns the reproductions of runID, oldest first; none is not an error.
func Links(home, runID string) ([]Link, error) {
	p, err := linkPath(home, runID)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fail(evidence.CodeWorktree, "cannot read %s: %v", p, err)
	}
	defer func() { _ = f.Close() }()
	var out []Link
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var l Link
		if json.Unmarshal(sc.Bytes(), &l) == nil && l.Schema == LinkSchema {
			out = append(out, l)
		}
	}
	return out, sc.Err()
}
