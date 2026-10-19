package main

// Purpose: `nself-ci-agent userns-probe` tells the node lifecycle (`nself ci nodes add`) whether this
// user-mode Linux host can separate a job from the agent with an unprivileged user namespace (D-1084).
// Inputs:  no arguments; /etc/subuid and the kernel's namespace settings.
// Outputs: one JSON line, hygiene.Probe (`{"available":true,"reason":"userns.ok"}`); exit 0 whenever the
//
//	probe ran (even when separation is unavailable), 1 when it could not run, 2 on usage.
//
// Constraints: fails closed. Off Linux, with no subordinate UID range or when the namespace cannot be
// made, it prints available:false. The probe file lives in the OS temp dir, which the job's mapped
// UID can traverse, so only the file's 0600 mode can deny the read (see hygiene.ProbeUserns).

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"runtime"

	"github.com/nself-org/plugins/free/ci/internal/hygiene"
)

// usernsProbeFor runs the probe; tests replace it.
var usernsProbeFor = hostUsernsProbe

func init() {
	must(RegisterSubcommand("userns-probe", "print whether unprivileged user namespaces separate jobs (JSON)", func(ctx context.Context, args []string, sio IO) int {
		if len(args) != 0 {
			fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent userns-probe")
			return 2
		}
		p, err := usernsProbeFor(ctx)
		if err != nil {
			fmt.Fprintf(sio.Stderr, "nself-ci-agent: userns-probe: %v\n", err)
			return 1
		}
		b, err := json.Marshal(p)
		if err != nil {
			fmt.Fprintf(sio.Stderr, "nself-ci-agent: userns-probe: %v\n", err)
			return 1
		}
		fmt.Fprintf(sio.Stdout, "%s\n", b)
		return 0
	}))
}

// hostUsernsProbe reads the agent user's first subordinate UID and runs hygiene.ProbeUserns.
func hostUsernsProbe(ctx context.Context) (hygiene.Probe, error) {
	if runtime.GOOS != "linux" {
		return hygiene.Probe{Reason: hygiene.ProbeNotLinux}, nil
	}
	sub := 0 // 0 makes ProbeUserns report userns.no_subuid
	if u, err := user.Current(); err == nil {
		if f, err := os.Open("/etc/subuid"); err == nil {
			defer f.Close()
			uid := os.Getuid()
			if start, _, ok := hygiene.ParseSubuid(f, u.Username, uid); ok {
				sub = start
			}
		}
	}
	return hygiene.ProbeUserns(ctx, os.TempDir(), sub)
}
