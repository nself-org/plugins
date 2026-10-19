package lifecycle

// Purpose: the production environment of the lifecycle commands: the store under $NSELF_CI_HOME, the
// admin identity and the current project.
// Inputs:  NSELF_CI_HOME (default ~/.nself/ci), the project resolver of the composition root.
// Outputs: Env.Open returns Deps and a close function.
// Constraints: the store is opened with a short retry on a busy database (another process holds the
//          writer); the actor is "cli:<user>", never "agent" or "probe".

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// CIHome is $NSELF_CI_HOME or ~/.nself/ci.
func CIHome() string {
	if v := os.Getenv("NSELF_CI_HOME"); v != "" {
		return v
	}
	u, _ := os.UserHomeDir()
	return filepath.Join(u, ".nself", "ci")
}

func openStore(path string) (*store.Store, error) {
	for end := time.Now().Add(10 * time.Second); ; time.Sleep(25 * time.Millisecond) {
		st, err := store.Open(path, store.Options{})
		busy := err != nil && (strings.Contains(err.Error(), "SQLITE_BUSY") || strings.Contains(err.Error(), "database is locked"))
		if err == nil || !busy || time.Now().After(end) {
			return st, err
		}
	}
}

func adminName() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return "cli:" + u.Username
	}
	if v := os.Getenv("USER"); v != "" {
		return "cli:" + v
	}
	return "cli:admin"
}

// ProductionEnv opens the real store. project resolves the current project id; a --project flag wins.
func ProductionEnv(project func(ctx context.Context) string) Env {
	return Env{
		Out: os.Stdout, Err: os.Stderr,
		Open: func(ctx context.Context, override string) (*Deps, func(), error) {
			home := CIHome()
			st, err := openStore(filepath.Join(home, "state.db"))
			if err != nil {
				return nil, nil, err
			}
			p := override
			if p == "" && project != nil {
				p = project(ctx)
			}
			return NewDeps(st, home, p, adminName()), func() { _ = st.Close() }, nil
		},
	}
}
