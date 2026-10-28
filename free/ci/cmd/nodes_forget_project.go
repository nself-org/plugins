package main

// nodes forget-project removes an obsolete root from the local deploy-host exclusion cache.
// Access to the CI data directory is admin access, as for other local node commands.
import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

func forgetProjectFlags(out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("nodes forget-project", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.Usage = func() {
		fmt.Fprintln(out, "usage: nself ci nodes forget-project <absolute-project-root> (local, audited)")
	}
	return fs
}

func init() {
	registerV15("nodes forget-project", subcommandEntry{
		V15Only: true,
		Flags:   func() *flag.FlagSet { return forgetProjectFlags(io.Discard) },
		Handler: func(args []string) int { return forgetProject(args, os.Stdout, os.Stderr) },
	})
}

func forgetProject(args []string, out, errOut io.Writer) int {
	fs := forgetProjectFlags(errOut)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if fs.NArg() != 1 || !filepath.IsAbs(fs.Arg(0)) {
		fs.Usage()
		return 1
	}
	// Do not stat or resolve symlinks: the root may have been deleted. Use its recorded absolute path.
	root := filepath.Clean(fs.Arg(0))
	home := lifecycle.CIHome()
	st, err := deployguard.OpenStore(filepath.Join(home, "state.db"))
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	defer st.Close()
	if err := newGuard(st, home, adminActor()).ForgetProject(context.Background(), root); err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	fmt.Fprintf(out, "forgot deploy-host exclusion project %s; remove it from ci.project_roots in config.yaml if configured\n", root)
	return 0
}
