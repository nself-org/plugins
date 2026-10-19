package deployguard

// Purpose: the production RunFunc: one short process through the exec funnel with the project's inventory
// files staged into the private workspace, so `nself` finds its project root without the whole tree
// being copied.
// Inputs:  the project root and argv.
// Outputs: the process output (stdout and stderr merged) and exit code.
// Constraints: only `.nself/control-plane.yaml` and the root `.env` and `.env.*` files are staged (regular
//          files at most 1 MiB; a symlink is resolved, as nself would read it; anything else fails); 30 s timeout; no stdin; the inherited environment
//          carries the operator's NSELF_DEPLOY_HOST_* variables and real HOME for ssh -G and known_hosts.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

var runSeq atomic.Int64

const maxStaged = 1 << 20

// maxOutput caps what one nself call may return (a larger document is cut and then fails to parse).
const maxOutput = 4 << 20

// runTimeout bounds one nself call (30 s), as a plain duration.
const runTimeout = time.Duration(3e10)

// FunnelRun is the production RunFunc.
func FunnelRun(ctx context.Context, dir string, argv ...string) ([]byte, int, error) {
	if len(argv) == 0 {
		return nil, -1, fmt.Errorf("empty command")
	}
	sweepStaged()
	var buf strings.Builder
	var ex ciexec.Executor
	spec := ciexec.JobSpec{
		AttemptID: fmt.Sprintf("dg-%d-%d", time.Now().UnixNano(), runSeq.Add(1)), CoordinatorID: "deployguard",
		Job: model.Job{Kind: "static"}, Timeout: runTimeout, Command: argv,
		Output: func(b []byte) { buf.Write(b[:min(max(0, maxOutput-buf.Len()), len(b))]) },
	}
	if dir != "" {
		spec.BeforeJob = func(_ context.Context, ws *os.Root) error { return stage(dir, ws) }
	}
	res := ex.Run(ctx, spec)
	return []byte(buf.String()), res.ExitCode, res.Err
}

// stage copies the inventory files of root into the workspace. A file that is absent is skipped; a name
// that exists but cannot be staged (not a regular file after symlinks resolve, or over 1 MiB) fails the
// call, so a partial project is never read.
func stage(root string, ws *os.Root) error {
	names := []string{".nself/control-plane.yaml"}
	if ents, err := os.ReadDir(root); err == nil {
		for _, e := range ents {
			if n := e.Name(); n == ".env" || strings.HasPrefix(n, ".env.") {
				names = append(names, n)
			}
		}
	}
	for _, n := range names {
		if err := copyOne(root, ws, n); err != nil {
			return err
		}
	}
	return nil
}

func copyOne(root string, dst *os.Root, name string) error {
	real, err := filepath.EvalSymlinks(filepath.Join(root, name))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stage %s: %w", name, err)
	}
	fi, err := os.Stat(real)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxStaged {
		return fmt.Errorf("stage %s: not a regular file of at most 1 MiB", name)
	}
	in, err := os.Open(real)
	if err != nil {
		return fmt.Errorf("stage %s: %w", name, err)
	}
	defer func() { _ = in.Close() }()
	if d := filepath.Dir(name); d != "." {
		if err = dst.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	out, err := dst.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(in, maxStaged))
	return errors.Join(err, out.Close())
}
