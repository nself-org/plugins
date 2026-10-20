package darwin

// Purpose: the upload steps of a darwin Install: the sudoers drop-in, the agent binary, the LaunchDaemon,
// slot retirement and the Xcode warning. Every file is generated or chosen locally, copied to the ssh
// user's home and installed by a fixed script under sudo; nothing is written through a heredoc.
// Inputs:  the elevated runner and the bound options.
// Outputs: the files in place; Logf lines.
// Constraints: a local temp file is removed on return; a failed copy is an E669 step error naming the
//          step; the Xcode probe never fails an install.

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
)

var jobUserRe = regexp.MustCompile(`^` + agentUser + `-job-([1-9][0-9]{0,2})$`)

// surplusSlots returns the slot numbers above slots among the listed names, ascending and unique.
// Only accounts named nself-ci-job-<number> count; anything else is never passed to a script.
func surplusSlots(listed string, slots int) []int {
	seen := map[int]bool{}
	var out []int
	for _, name := range strings.Fields(listed) {
		m := jobUserRe.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if n > slots && n <= 999 && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

func (f *Family) retireSurplus(ctx context.Context, el *elevated, o bootstrap.Options) error {
	listed, err := el.sh(ctx, ReasonUsers, "list-users", listJobUsersScript())
	if err != nil {
		return err
	}
	for _, n := range surplusSlots(listed, o.Slots) {
		out, rerr := el.sh(ctx, ReasonUsers, "retire-slot", removeSlotScript(n))
		if rerr != nil {
			return rerr
		}
		o.Logf("bootstrap slots: %s %s", jobUser(n), lastLine(out))
	}
	return nil
}

// stage writes content to a private local file and copies it to the node under name.
func stage(ctx context.Context, o bootstrap.Options, reason, step, name, content string) error {
	dir, err := os.MkdirTemp("", "nself-ci-darwin-")
	if err != nil {
		return bootstrap.NewError(reason, "bootstrap step %s failed: %s", step, bootstrap.Detail(err))
	}
	defer os.RemoveAll(dir)
	local := filepath.Join(dir, name)
	if err = os.WriteFile(local, []byte(content), 0o600); err != nil {
		return bootstrap.NewError(reason, "bootstrap step %s failed: %s", step, bootstrap.Detail(err))
	}
	if err = o.Copy(ctx, local, name); err != nil {
		return bootstrap.NewError(reason, "bootstrap step %s failed: %s", step, bootstrap.Detail(err))
	}
	return nil
}

func (f *Family) installSudoers(ctx context.Context, el *elevated, o bootstrap.Options) error {
	if err := stage(ctx, o, ReasonSudoers, "sudoers-upload", SudoersStage, Sudoers(o.Slots)); err != nil {
		return err
	}
	if _, err := el.sh(ctx, ReasonSudoers, "sudoers", sudoersScript()); err != nil {
		return err
	}
	o.Logf("bootstrap sudoers: done")
	return nil
}

// installAgent uploads and installs the binary when the caller gave one. It is owned by the agent user,
// so `nodes upgrade` replaces it atomically without root.
func (f *Family) installAgent(ctx context.Context, el *elevated, o bootstrap.Options) error {
	if o.AgentBinary == "" {
		return nil
	}
	const uploaded = ".nself-ci-agent.upload"
	if err := o.Copy(ctx, o.AgentBinary, uploaded); err != nil {
		return bootstrap.NewError(bootstrap.ReasonStep, "bootstrap step agent-upload failed: %s", bootstrap.Detail(err))
	}
	if _, err := el.sh(ctx, bootstrap.ReasonStep, "agent", agentScript(uploaded)); err != nil {
		return err
	}
	o.Logf("bootstrap agent: installed %s", agentPath)
	return nil
}

func (f *Family) installService(ctx context.Context, el *elevated, o bootstrap.Options) error {
	if err := stage(ctx, o, ReasonLaunchd, "plist-upload", PlistStage, Plist()); err != nil {
		return err
	}
	if _, err := el.sh(ctx, ReasonLaunchd, "launchd", plistScript()); err != nil {
		return err
	}
	o.Logf("bootstrap launchd: %s loaded", Label)
	return nil
}

// warnXcode logs the E669 xcode_license warning when the license is not accepted. It never fails.
func (f *Family) warnXcode(ctx context.Context, el *elevated, o bootstrap.Options) {
	out, err := el.sh(ctx, ReasonXcodeLicense, "xcode", xcodeProbeScript(f.XcodePathPrefix))
	if err != nil {
		return
	}
	if w := xcodeWarning(lastLine(out)); w != "" {
		o.Logf("%s", w)
	}
}
