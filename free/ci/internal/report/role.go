package report

// Purpose: the process role (P7-NODE-10, Epic D7): whether this process is a PERSONAL run or the TEAM
// coordinator. `nself ci serve` in V15 mode sets RoleTeam at start; P7-HOST-02 reads it to pick the status
// reporting owner.
// Inputs:  SetProcessRole(Role) from the serve command.
// Outputs: ProcessRole(), RolePersonal until a TEAM coordinator starts.
// Constraints: the zero value is RolePersonal; the role lives in an atomic, so it is safe to read from
//          any goroutine; nothing else in this package depends on it.

import "sync/atomic"

// Role is the role of this process.
type Role int

// The roles. RolePersonal is the zero value and the default.
const (
	RolePersonal Role = iota
	RoleTeam
)

// String names the role.
func (r Role) String() string {
	if r == RoleTeam {
		return "team"
	}
	return "personal"
}

var processRole atomic.Int32

// SetProcessRole sets the role of this process.
func SetProcessRole(r Role) { processRole.Store(int32(r)) }

// ProcessRole returns the role of this process.
func ProcessRole() Role { return Role(processRole.Load()) }
