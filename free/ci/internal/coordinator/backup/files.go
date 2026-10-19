package backup

// Purpose: the file writes of a restore: atomic installs, the pre-restore copies and the rollback command.
// Inputs:  paths in the data dir and the bytes to install.
// Outputs: files written 0600 through a temp file and rename; a shell line that undoes a restore.
// Constraints: nothing is written before Read has authenticated the archive; a pre-restore copy is created
//          O_EXCL so an earlier copy is never overwritten; the rollback line quotes every path.

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// writeFileAtomic installs data at path (0600) through a temp file in the same directory.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".restore-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	if err = f.Chmod(0o600); err == nil {
		if _, err = f.Write(data); err == nil {
			err = f.Sync()
		}
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}

// copyAside saves the file at path as path+suffix when it exists and returns the new name ("" when none).
func copyAside(path, suffix string) (string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	to := path + suffix
	f, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return to, err
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// rollbackLine is the command that puts the saved files back; stop the coordinator first.
func rollbackLine(dataDir string, saved map[string]string) string {
	if len(saved) == 0 {
		return "" // the dir held nothing: there is nothing to put back
	}
	var cmds []string
	for orig, copied := range saved {
		cmds = append(cmds, "mv -f "+shQuote(copied)+" "+shQuote(orig))
	}
	sort.Strings(cmds)
	db := filepath.Join(dataDir, nameState)
	cmds = append(cmds, "rm -f "+shQuote(db+"-wal")+" "+shQuote(db+"-shm")+" "+shQuote(pendingPath(dataDir))) // the marker goes with a rollback
	return strings.Join(cmds, " && ")
}

func sortStrings(s []string) { sort.Strings(s) }

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func btoa(b bool) string { return strconv.FormatBool(b) }

// rollbackHint is the suffix of an error message that names the rollback line (empty when nothing was saved).
func rollbackHint(line string) string {
	if line == "" {
		return ""
	}
	return " (rollback, with the coordinator stopped: " + line + ")"
}
