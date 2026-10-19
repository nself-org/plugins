package deployguard

// Purpose: the production RunFunc: one short process through the exec funnel with the project's inventory
// files staged into the private workspace, so `nself` finds its project root without the whole tree
// being copied.
// Inputs:  the project root and argv.
// Outputs: the process output (stdout and stderr merged) and exit code.
// Constraints: only `.nself/control-plane.yaml` and the root `.env` and `.env.*` files are staged (regular
//          files, at most 1 MiB, no symlink followed); 30 s timeout; no stdin; the inherited environment
//          carries the operator's NSELF_DEPLOY_HOST_* variables and real HOME for ssh -G and known_hosts.

import (
	"context"
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

// FunnelRun is the production RunFunc.
func FunnelRun(ctx context.Context, dir string, argv ...string) ([]byte, int, error) {
	if len(argv) == 0 {
		return nil, -1, fmt.Errorf("empty command")
	}
	var buf strings.Builder
	var ex ciexec.Executor
	spec := ciexec.JobSpec{
		AttemptID: fmt.Sprintf("dg-%d-%d", time.Now().UnixNano(), runSeq.Add(1)), CoordinatorID: "deployguard",
		Job: model.Job{Kind: "static"}, Timeout: 30 * time.Second, Command: argv,
		Output: func(b []byte) { buf.Write(b) },
	}
	if dir != "" {
		spec.BeforeJob = func(_ context.Context, ws *os.Root) error { return stage(dir, ws) }
	}
	res := ex.Run(ctx, spec)
	return []byte(buf.String()), res.ExitCode, res.Err
}

// stage copies the inventory files of root into the workspace.
func stage(root string, ws *os.Root) error {
	src, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	names := []string{".nself/control-plane.yaml"}
	if ents, err := os.ReadDir(root); err == nil {
		for _, e := range ents {
			if n := e.Name(); n == ".env" || strings.HasPrefix(n, ".env.") {
				names = append(names, n)
			}
		}
	}
	for _, n := range names {
		if err := copyOne(src, ws, n); err != nil {
			return err
		}
	}
	return nil
}

func copyOne(src, dst *os.Root, name string) error {
	fi, err := src.Lstat(name)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxStaged {
		return nil // absent, a link or too big: not staged
	}
	in, err := src.Open(name)
	if err != nil {
		return nil
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
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
