//go:build integration

package reliability

// Purpose: what the tests read off a node: the fixture job's start, beat and side-effect files, whether its process
//          and the agent are alive, and the agent's own log. These are the process timestamps and the
//          side-effect count the acceptance lines assert.
// Inputs:  a node index.
// Outputs: counts and nanosecond timestamps taken inside the container (one clock per node).
// Constraints: tests only; a paused or partitioned node answers nothing, so the alive probes tolerate a failing
//          docker exec and report false.

import (
	"strconv"
	"strings"
	"time"
)

// index is the position of a coordinator-side node id in the fleet.
func (l *lab) index(id string) int {
	for i, v := range l.ids {
		if v == id {
			return i
		}
	}
	l.t.Fatalf("no node %q in the lab (%v)", id, l.ids)
	return -1
}

// lines counts the lines of a node file (0 when it does not exist yet).
func (l *lab) lines(i int, file string) int {
	l.t.Helper()
	n, _ := strconv.Atoi(l.sh(i, "cat "+file+" 2>/dev/null | wc -l"))
	return n
}

// starts is how many times the fixture job began on node i.
func (l *lab) starts(i int) int { return l.lines(i, "/tmp/starts") }

// sideEffects is how many lines the fixture job wrote to node i's side-effect file.
func (l *lab) sideEffects(i int) int { return l.lines(i, "/tmp/side-effect") }

// fleetSideEffects sums the side-effect lines over every node: the job's effect must be written at most once.
func (l *lab) fleetSideEffects() int {
	l.t.Helper()
	n := 0
	for i := range l.names {
		n += l.sideEffects(i)
	}
	return n
}

// fleetStarts sums how many times the job began over every node.
func (l *lab) fleetStarts() int {
	l.t.Helper()
	n := 0
	for i := range l.names {
		n += l.starts(i)
	}
	return n
}

// lastBeat is the time (node clock) of the fixture job's last beat on node i; zero when it never beat.
func (l *lab) lastBeat(i int) time.Time {
	l.t.Helper()
	ns, _ := strconv.ParseInt(l.sh(i, "tail -n 1 /tmp/beats 2>/dev/null || true"), 10, 64)
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

// jobAlive reports whether the fixture job's shell runs in node i (false when the node cannot be asked).
func (l *lab) jobAlive(i int) bool {
	out, err := l.tryExec(i, `grep -al '[/]tmp/beats' /proc/[0-9]*/cmdline | head -n 1`)
	return err == nil && strings.TrimSpace(out) != ""
}

// agentAlive reports whether the agent process runs in node i.
func (l *lab) agentAlive(i int) bool {
	out, err := l.tryExec(i, `kill -0 $(cat /tmp/agent.pid) 2>/dev/null && echo up || echo down`)
	return err == nil && out == "up"
}

// killAgent sends SIGKILL to the agent of node i (the job it started is not touched).
func (l *lab) killAgent(i int) { l.sh(i, "kill -9 $(cat /tmp/agent.pid)") }

// agentLog is the agent's log of node i.
func (l *lab) agentLog(i int) string {
	out, _ := l.tryExec(i, "cat /tmp/agent.log")
	return out
}

// pause freezes every process of node i (docker pause: SIGSTOP for the whole container); unpause thaws it.
func (l *lab) pause(i int)   { l.f.Pause(l.t, l.names[i]) }
func (l *lab) unpause(i int) { l.f.Unpause(l.t, l.names[i]) }

// partition detaches node i from the fleet network; heal puts it back.
func (l *lab) partition(i int) { l.f.Partition(l.t, l.names[i]) }
func (l *lab) heal(i int)      { l.f.Heal(l.t, l.names[i]) }

// agentReachable reports whether node i answers docker exec and can open a connection to the coordinator.
func (l *lab) agentReachable(i int) bool {
	out, err := l.tryExec(i, "timeout 2 bash -c 'exec 3<>/dev/tcp/"+l.host+"/"+itoa(l.c.port)+"' && echo ok || echo no")
	return err == nil && out == "ok"
}
