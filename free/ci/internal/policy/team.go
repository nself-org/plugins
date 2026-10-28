package policy

// Purpose: where the team id of a resolve comes from (D-1284). A TEAM coordinator is one team; its team
// policy rows apply to every run, plan and dispatch it serves.
// Inputs:  NSELF_CI_TEAM from the operator's own environment (the coordinator process, or the shell that runs
//          `nself ci`); never from a job's env or a repository file.
// Outputs: the team subject for the team policy scope; empty means no team rows apply.
// Constraints: the id uses the same alphabet as `policy set --team`. A set but malformed value is an error,
//          not "no team", so a typo cannot silently drop a team's tighten-only rows. An explicit Repo.Team
//          (`policy show --team`) wins over the environment.

import (
	"os"
	"strings"
)

// TeamEnv names the environment variable that carries the coordinator's team id.
const TeamEnv = "NSELF_CI_TEAM"

// TeamFromEnv returns the team id named by NSELF_CI_TEAM, or "" when it is unset or blank.
func TeamFromEnv() (string, error) {
	v := strings.TrimSpace(os.Getenv(TeamEnv))
	if v == "" {
		return "", nil
	}
	if !teamID.MatchString(v) {
		return "", refuse("%s is not a team id of letters, digits, dot, dash and underscore (at most 64)", TeamEnv)
	}
	return v, nil
}

// teamOf is the team subject for repo: the explicit one, else the coordinator's.
func teamOf(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	return TeamFromEnv()
}
