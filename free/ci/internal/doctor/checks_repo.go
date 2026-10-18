package doctor

// Purpose: the repository and state checks: the store (opens, version),
// `.nself/ci.yaml` (validates), a leftover `.ci.yaml`, and a git-ignored
// `.nself/ci.yaml`.
// Inputs:  the repository root, Deps.
// Outputs: Check values CI-D004–CI-D008, and the loaded config for the
// environment checks.
// Constraints: doctor never creates the state database: a missing file is a
// pass ("created on first run"); the open of an existing one runs the same
// migrations `nself ci runs` does.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// errNewerReader is the store code for a database that needs a newer binary.
const errNewerReader = "E606"

func checkStore(d Deps) []Check {
	opens := Check{ID: "CI-D004", Name: "store opens"}
	version := Check{ID: "CI-D005", Name: "store version"}
	path, err := StorePath(d)
	if err != nil {
		opens.Status, opens.Message = Warn, "cannot locate the state directory: "+err.Error()
		opens.Remediation = "Set NSELF_CI_HOME to a writable directory"
		version.Status, version.Message = Pass, "not checked"
		return []Check{opens, version}
	}
	if _, err = os.Stat(path); errors.Is(err, os.ErrNotExist) {
		opens.Status, opens.Message = Pass, "no store yet at "+path+" (created on first run)"
		version.Status, version.Message = Pass, "no store yet"
		return []Check{opens, version}
	}
	err = d.OpenStore(path)
	var se *store.Error
	switch {
	case err == nil:
		opens.Status, opens.Message = Pass, "store opens and migrations are current ("+path+")"
		version.Status, version.Message = Pass, "store version is supported by this binary"
	case errors.As(err, &se) && se.Code == errNewerReader:
		opens.Status, opens.Message = Pass, "store file is readable ("+path+")"
		version.Status, version.Message = Fail, "the store is newer than this binary"
		version.Detail = err.Error()
		version.Remediation = "Upgrade nself-ci (nself plugin update ci) before opening this store"
	case isBusy(err):
		opens.Status, opens.Message = Warn, "the store is busy: another nself ci process holds it ("+path+")"
		opens.Detail = err.Error()
		opens.Remediation = "Wait for the running nself ci command to finish, then re-run doctor"
		version.Status, version.Message = Pass, "not checked: the store is busy"
	default:
		opens.Status, opens.Message = Fail, "the store does not open ("+path+")"
		opens.Detail = err.Error()
		opens.Remediation = "If the file is damaged, move " + path + " aside; the next run creates a fresh store (run history is lost)"
		version.Status, version.Message = Pass, "not checked: the store did not open"
	}
	return []Check{opens, version}
}

// isBusy reports a transient SQLite lock, which is not a damaged store.
func isBusy(err error) bool {
	return strings.Contains(err.Error(), "SQLITE_BUSY") || strings.Contains(err.Error(), "database is locked")
}

func checkConfig(root string) (Check, *model.PipelineConfig) {
	c := Check{ID: "CI-D006", Name: "config valid"}
	path := filepath.Join(root, config.FileName)
	loaded, err := config.Load(path)
	var ce *config.Error
	switch {
	case err == nil:
		c.Status, c.Message = Pass, config.FileName+" is valid"
		if len(loaded.Warnings) > 0 {
			texts := make([]string, len(loaded.Warnings))
			for i, w := range loaded.Warnings {
				texts[i] = w.Code + ": " + w.Message
			}
			c.Status, c.Message = Warn, config.FileName+" is valid with warnings"
			c.Detail = strings.Join(texts, "\n")
			c.Remediation = "Fix the named keys so every required job always runs"
		}
		return c, &loaded.Config
	case errors.Is(err, os.ErrNotExist):
		c.Status, c.Message = Pass, config.FileName+" is absent: detected defaults apply"
	case errors.As(err, &ce):
		c.Status, c.Message = Fail, config.FileName+" is invalid"
		c.Detail = ce.Error()
		c.Remediation = "Fix the named key at the reported file:line (`nself ci config show` re-checks it)"
	default:
		c.Status, c.Message = Fail, config.FileName+" cannot be read: "+err.Error()
		c.Remediation = "Check the file's permissions and that it is a regular file"
	}
	return c, nil
}

func checkLeftover(root string) Check {
	c := Check{ID: "CI-D007", Name: "no leftover .ci.yaml"}
	found, err := config.Discover(root)
	switch {
	case err != nil:
		c.Status, c.Message = Warn, "could not search for .ci.yaml: "+err.Error()
		c.Remediation = "Run doctor from a readable repository root (--repo)"
	case len(found) > 0:
		c.Status, c.Message = Fail, "legacy .ci.yaml present under V15: "+strings.Join(found, ", ")
		c.Remediation = "Run `nself ci config migrate --write`, then delete the old file"
	default:
		c.Status, c.Message = Pass, "no legacy .ci.yaml"
	}
	return c
}

func checkIgnored(ctx context.Context, root string, d Deps) Check {
	c := Check{ID: "CI-D008", Name: "config not git-ignored"}
	if _, err := os.Stat(filepath.Join(root, config.FileName)); err != nil {
		c.Status, c.Message = Pass, config.FileName+" is absent: nothing to commit"
		return c
	}
	run := func(ctx context.Context, argv []string) (int, error) {
		_, exit, err := d.Capture(ctx, argv...)
		return exit, err
	}
	w, err := config.CheckIgnored(ctx, root, run)
	switch {
	case err != nil:
		c.Status, c.Message = Warn, "could not ask git whether "+config.FileName+" is ignored: "+err.Error()
		c.Remediation = "Install git, or check .gitignore by hand"
	case w != nil:
		c.Status, c.Message = Warn, config.FileName+" is git-ignored, so it would never be committed"
		c.Remediation = "In .gitignore replace a `.nself/` line with `.nself/*` and `!.nself/ci.yaml`"
	default:
		c.Status, c.Message = Pass, config.FileName+" is not git-ignored"
	}
	return c
}
