package detach

// Purpose: Start re-executes the plugin binary as a detached ordinary run
// (P7-CI-63, Epic D30) and returns once the run is queued. The child is the
// normal engine (a PERSONAL coordinator, or a --coordinator client), so cancel,
// stale-coordinator recovery and evidence work unchanged.
// Inputs:  Params (executable, the user's argv, state directory, environment).
// Outputs: contract:ci.run-handle v1, or E648 (spawn, no_queued_event,
// child_exit).
// Constraints: no shell between parent and child; the child environment is the
// parent's minus NSELF_MCP_TOKEN; the stream file is 0600 in a 0700 directory;
// the wait for the first `queued` record is bounded (30 s); a child that never
// announces a run is terminated, one that ended early is reported with the
// redacted tail of its stream.

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
)

// QueuedWait is how long Start waits for the first `queued` record.
const QueuedWait = 30 * time.Second

// Params is what Start needs; zero fields take the production default.
type Params struct {
	Exe  string   // plugin binary (os.Executable)
	Args []string // the user's argv after `nself ci`, still holding --detach
	Home string   // state directory ("" = NSELF_CI_HOME or ~/.nself/ci)
	Env  []string // parent environment (os.Environ)
	Wait time.Duration
	Poll time.Duration
	// Spawn and Stop replace the process layer in tests.
	Spawn func(argv, env []string, logPath string) (*ciexec.Detached, error)
	Stop  func(pid int) error
	// Verify confirms that a run id announced in the stream is a real run (the
	// stream file also holds the child's stderr, so a `queued` line alone proves
	// nothing). nil accepts any well-formed record; a remote run cannot be
	// verified from the local store and passes nil.
	Verify func(runID string) bool
}

// Start spawns the child and waits for its run to be queued.
func Start(p Params) (Handle, error) {
	home := Home(p.Home)
	if home == "" || p.Exe == "" {
		return Handle{}, fail(ReasonSpawn, nil, "no state directory or executable to start the run from")
	}
	wait, poll := p.Wait, p.Poll
	if wait <= 0 {
		wait = QueuedWait
	}
	if poll <= 0 {
		poll = 20 * time.Millisecond
	}
	spawn, stop := p.Spawn, p.Stop
	if spawn == nil {
		spawn = ciexec.StartDetached
	}
	if stop == nil {
		stop = ciexec.TerminateDetached
	}
	tok := make([]byte, 8)
	if _, err := rand.Read(tok); err != nil {
		return Handle{}, fail(ReasonSpawn, err, "no random token for the stream file: %v", err)
	}
	path := filepath.Join(Dir(home), hex.EncodeToString(tok)+".ndjson")
	d, err := spawn(append([]string{p.Exe}, ChildArgs(p.Args)...), ChildEnv(p.Env), path)
	if err != nil {
		return Handle{}, spawnError(err)
	}
	runID, err := awaitQueued(path, d.Exited, wait, poll, p.Verify)
	if err != nil {
		var de *Error
		if errors.As(err, &de) {
			de.Tail = RedactedTail(path, p.Env)
			if de.Reason == ReasonNoQueued {
				_ = stop(d.PID)
			}
		}
		_ = os.Remove(path) // the tail is in the error; the raw child output must not stay on disk
		return Handle{}, err
	}
	if final := filepath.Join(Dir(home), runID+".ndjson"); ValidRunID(runID) && os.Rename(path, final) == nil {
		path = final // the child keeps writing: its descriptor follows the rename
	}
	return NewHandle(runID, d.PID, CoordinatorOf(p.Args, envLookup(p.Env)), path, JobOf(p.Args)), nil
}

func spawnError(err error) *Error {
	var ee *ciexec.Error
	if errors.As(err, &ee) {
		return fail(ReasonSpawn, err, "%s", strings.TrimPrefix(ee.Message, "spawn: "))
	}
	return fail(ReasonSpawn, err, "%v", err)
}

// awaitQueued polls the stream file for the first record that names a run.
func awaitQueued(path string, exited <-chan struct{}, wait, poll time.Duration, verify func(string) bool) (string, error) {
	r := &Reader{Path: path}
	defer func() { _ = r.Close() }()
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	tick := time.NewTicker(poll)
	defer tick.Stop()
	ended := false
	for {
		lines, err := r.Next()
		if err != nil {
			return "", fail(ReasonSpawn, err, "reading the stream file: %v", err)
		}
		for _, l := range lines {
			if rec, ok := ParseLine(l); ok && rec.Type == "queued" && rec.Seq == 1 && ValidRunID(rec.RunID) && (verify == nil || verify(rec.RunID)) {
				return rec.RunID, nil
			}
		}
		if ended {
			return "", fail(ReasonChildExit, nil, "the detached run ended before it was queued")
		}
		select {
		case <-exited:
			ended = true // read once more: the last lines may have landed just before exit
			exited = nil
		case <-deadline.C:
			return "", fail(ReasonNoQueued, nil, "the detached run did not report queued within %s", wait)
		case <-tick.C:
		}
	}
}
