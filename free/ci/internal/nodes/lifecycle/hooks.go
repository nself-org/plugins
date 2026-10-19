package lifecycle

// Purpose: the extension registry later Tickets use to hang behaviour on the node lifecycle (review F6):
// add/pair/test hooks, agent installers, extra flags, rotate modes, revokers and session closers.
// Inputs:  Register* calls from a package's init().
// Outputs: the registered functions, read by Add, Test, Rotate and Revoke.
// Constraints: a duplicate installer or rotate-mode name, an unknown hook point and a nil function
//          panic at init (a programming error); registration order is run order; the registry is
//          guarded by a mutex because tests register at run time.

import (
	"context"
	"flag"
	"sync"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Point is where an AddHook runs.
type Point string

// Hook points: preadd (D-1219: after the destination is parsed, before any contact with the host), add (after
// the probe, before the node is registered), pair (P7-NODE-07's pairing), test.
const (
	PointPreAdd Point = "preadd"
	PointAdd    Point = "add"
	PointPair   Point = "pair"
	PointTest   Point = "test"
)

// HookEvent is what a hook sees. Flags is the parsed flag set of the command, including the flags
// RegisterFlags added.
type HookEvent struct {
	Point   Point
	NodeID  string
	Project string
	Flags   *flag.FlagSet
	// Dest is the destination as typed (`nodes add`); empty at the other points.
	Dest string
	// Capability is the draft a node is registered with (`nodes add`, after the probe); nil elsewhere, including
	// PointPreAdd, where nothing is known about the host yet.
	// An add hook may set its DeployHost (P7-NODE-14) and nothing else.
	Capability *model.Capability
}

// AddHook runs at a Point; an error from an add hook fails `nodes add` before the node is registered
// (the pinned key is rolled back), so a failed hook leaves no node and no pin.
type AddHook func(ctx context.Context, e HookEvent) error

// InstallRequest is one agent installation.
type InstallRequest struct {
	NodeID   string
	Target   remote.Target
	Platform Platform
	Dir      string // relative to the SSH user's home: .nself/ci-agent
	Source   AgentSource
	Flags    *flag.FlagSet
}

// AgentInstaller puts the agent binary on the node. The built-in "local-file" installer copies
// --agent-binary; the signed Release source (internal/agent/install) is the default when none is given.
type AgentInstaller func(ctx context.Context, d *Deps, r InstallRequest) error

// DefaultSourceFunc builds the AgentSource `nodes add` uses when the admin gave no --agent-binary and the node has no agent yet.
type DefaultSourceFunc func() AgentSource

// FlagsFunc adds flags to a subcommand's flag set (the subcommand is "add", "test", ...).
type FlagsFunc func(fs *flag.FlagSet)

// RotateRequest is one `nodes rotate` run in a mode.
type RotateRequest struct {
	NodeID string
	// Target is the pinned ssh target of the node (zero for the host-key mode when the pin is lost).
	Target remote.Target
	// ScanDest is the bare destination the host-key scan connects to.
	ScanDest string
	// HostKey is the --host-key value ("" when none); Flags may be nil.
	HostKey string
	Flags   *flag.FlagSet
}

// RotateMode rotates one credential of a node (the built-in "host-key" mode needs --host-key).
type RotateMode func(ctx context.Context, d *Deps, r RotateRequest) error

// SessionCloser closes live sessions of a revoked node after the revoke commit (P7-NODE-20).
type SessionCloser func(ctx context.Context, nodeID string) error

type extensions struct {
	mu         sync.Mutex
	hooks      map[Point][]AddHook
	installers map[string]AgentInstaller
	flags      map[string][]FlagsFunc
	modes      map[string]RotateMode
	revokers   []store.RevokeHook
	closers    []SessionCloser
	defSource  DefaultSourceFunc
}

var ext = newExtensions()

func newExtensions() *extensions {
	return &extensions{hooks: map[Point][]AddHook{}, installers: map[string]AgentInstaller{}, flags: map[string][]FlagsFunc{}, modes: map[string]RotateMode{}}
}

// RegisterAddHook runs fn at point.
func RegisterAddHook(point Point, fn AddHook) {
	if fn == nil || (point != PointPreAdd && point != PointAdd && point != PointPair && point != PointTest) {
		panic("nself-ci: invalid lifecycle hook " + string(point))
	}
	ext.mu.Lock()
	defer ext.mu.Unlock()
	ext.hooks[point] = append(ext.hooks[point], fn)
}

// RegisterAgentInstaller names an installer; `nodes add --installer <name>` selects it.
func RegisterAgentInstaller(name string, fn AgentInstaller) {
	ext.mu.Lock()
	defer ext.mu.Unlock()
	if _, dup := ext.installers[name]; dup || name == "" || fn == nil {
		panic("nself-ci: invalid agent installer " + name)
	}
	ext.installers[name] = fn
}

// RegisterDefaultSource sets the install source `nodes add` falls back to without --agent-binary (P7-NODE-15: the signed
// Release source). It registers once; a second call or a nil function panics at init.
func RegisterDefaultSource(fn DefaultSourceFunc) {
	ext.mu.Lock()
	defer ext.mu.Unlock()
	if fn == nil || ext.defSource != nil {
		panic("nself-ci: invalid default agent source registration")
	}
	ext.defSource = fn
}

// RegisterFlags lets a later Ticket add flags to a subcommand.
func RegisterFlags(subcommand string, fn FlagsFunc) {
	if subcommand == "" || fn == nil {
		panic("nself-ci: invalid lifecycle flags registration")
	}
	ext.mu.Lock()
	defer ext.mu.Unlock()
	ext.flags[subcommand] = append(ext.flags[subcommand], fn)
}

// RegisterRotateMode names a rotate mode; `nodes rotate --mode <name>` selects it.
func RegisterRotateMode(name string, fn RotateMode) {
	ext.mu.Lock()
	defer ext.mu.Unlock()
	if _, dup := ext.modes[name]; dup || name == "" || fn == nil {
		panic("nself-ci: invalid rotate mode " + name)
	}
	ext.modes[name] = fn
}

// RegisterRevoker adds a hook that runs inside the revoke transaction (the pairing deny list).
func RegisterRevoker(fn store.RevokeHook) {
	if fn == nil {
		panic("nself-ci: nil revoker")
	}
	ext.mu.Lock()
	defer ext.mu.Unlock()
	ext.revokers = append(ext.revokers, fn)
}

// RegisterSessionCloser adds a hook that closes live sessions after the revoke commit.
func RegisterSessionCloser(fn SessionCloser) {
	if fn == nil {
		panic("nself-ci: nil session closer")
	}
	ext.mu.Lock()
	defer ext.mu.Unlock()
	ext.closers = append(ext.closers, fn)
}

func (e *extensions) hooksAt(p Point) []AddHook {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]AddHook(nil), e.hooks[p]...)
}

func (e *extensions) installer(name string) (AgentInstaller, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	fn, ok := e.installers[name]
	return fn, ok
}

func (e *extensions) defaultSource() AgentSource {
	e.mu.Lock()
	fn := e.defSource
	e.mu.Unlock()
	if fn == nil {
		return nil
	}
	return fn()
}

func (e *extensions) mode(name string) (RotateMode, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	fn, ok := e.modes[name]
	return fn, ok
}

func (e *extensions) flagFuncs(sub string) []FlagsFunc {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]FlagsFunc(nil), e.flags[sub]...)
}

func (e *extensions) revokerList() []store.RevokeHook {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]store.RevokeHook(nil), e.revokers...)
}

func (e *extensions) closerList() []SessionCloser {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]SessionCloser(nil), e.closers...)
}

// RunHooks runs the hooks registered at point in order and stops at the first error. P7-NODE-07's pairing
// calls it with PointPair; Add and Test call it for their own points.
func RunHooks(ctx context.Context, point Point, e HookEvent) error {
	e.Point = point
	for _, h := range ext.hooksAt(point) {
		if err := h(ctx, e); err != nil {
			return err
		}
	}
	return nil
}
