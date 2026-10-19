package bootstrap

// Purpose: the fixed layout of a system node (Epic D8, D20) and the options of an Install or Uninstall.
// Inputs:  Options from `nodes add --system`: the slot count, the ssh target and the remote seams.
// Outputs: the names and paths every family agrees on, so TRUST-09's job shim, the sudoers rules and the
//          service unit cannot drift apart.
// Constraints: user names are fixed (`nself-ci`, `nself-ci-job-<n>` for n = 1..slots); every remote
//          command is a fixed argv or a script built from these constants and integers, never from the
//          node's replies; no script holds a backslash (the sdk refuses one in an argument).

import (
	"context"
	"io"
	"strconv"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// The layout of a system node.
const (
	AgentUser = "nself-ci"
	// AgentDir holds the agent binary (world-executable so a job user can run the shim through sudo).
	AgentDir = "/opt/nself-ci-agent"
	// AgentPath is the agent binary the sudoers rules name.
	AgentPath = AgentDir + "/bin/nself-ci-agent"
	// StateDir holds the node key and the age identity: mode 0700, owned by the agent user alone.
	StateDir = "/var/lib/nself-ci-agent"
	// WorkDir is the agent's work base (executor work areas): traversable, not listable.
	WorkDir = "/var/lib/nself-ci-work"
	// JobsDir holds one 0700 job root per slot, owned by that slot's job user.
	JobsDir = "/var/lib/nself-ci-jobs"
	// LogDir is the agent's log directory.
	LogDir = "/var/log/nself-ci-agent"
	// SudoersDir receives one rule file per slot user.
	SudoersDir = "/etc/sudoers.d"
	// UnitPath is the systemd unit of pull mode.
	UnitPath = "/etc/systemd/system/nself-ci-agent.service"
	// MaxSlots bounds the job users one node gets.
	MaxSlots = 64
)

// JobUser is the account of slot n (n = 1..slots).
func JobUser(n int) string { return AgentUser + "-job-" + strconv.Itoa(n) }

// SudoersFile is the rule file of slot n.
func SudoersFile(n int) string { return SudoersDir + "/" + JobUser(n) }

// JobRoot is the job root of slot n.
func JobRoot(n int) string { return JobsDir + "/" + strconv.Itoa(n) }

// RunFunc runs one fixed argv on the node as the ssh user and returns its combined output.
type RunFunc func(ctx context.Context, argv ...string) (string, error)

// CopyFunc copies a local file to a path on the node (relative paths are in the ssh user's home).
type CopyFunc func(ctx context.Context, local, remotePath string) error

// Options are the inputs of Install and Uninstall.
type Options struct {
	// Target is the node. Install overwrites it with its own t argument.
	Target remote.Target
	// Slots is the number of job users (1..MaxSlots).
	Slots int
	// Run and Copy default to the sdk's RunArgv and CopyTo bound to Target.
	Run  RunFunc
	Copy CopyFunc
	// AgentBinary is a local file to install as AgentPath; empty leaves the binary alone.
	AgentBinary string
	// Logf receives one line per step ("users: created nself-ci-job-2"); nil discards them.
	Logf func(format string, args ...any)
	// Stdout receives the operator instructions of a family that cannot act (darwin without a family).
	Stdout io.Writer
}

// bind fills the defaults and validates o for an Install.
func (o Options) bind(t remote.Target, needSlots bool) (Options, error) {
	o.Target = t
	if needSlots && (o.Slots < 1 || o.Slots > MaxSlots) {
		return o, NewError(ReasonUsage, "--slots must be between 1 and %d", MaxSlots)
	}
	return o.withDefaults()
}

func (o Options) withDefaults() (Options, error) {
	if o.Run == nil {
		if o.Target.Dest == "" {
			return o, NewError(ReasonUsage, "no node to bootstrap")
		}
		t := o.Target
		o.Run = func(ctx context.Context, argv ...string) (string, error) { return remote.RunArgv(ctx, t, argv...) }
	}
	if o.Copy == nil {
		t := o.Target
		o.Copy = func(ctx context.Context, local, remotePath string) error {
			return remote.CopyTo(ctx, t, local, remotePath)
		}
	}
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	return o, nil
}

// slotList is "1 2 3" for the shell loops of the scripts.
func slotList(slots int) string {
	s := ""
	for n := 1; n <= slots; n++ {
		if n > 1 {
			s += " "
		}
		s += strconv.Itoa(n)
	}
	return s
}

func stepErr(step string, err error) *Error {
	return NewError(ReasonStep, "bootstrap step %s failed: %s", step, Detail(err))
}
