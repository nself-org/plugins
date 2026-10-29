package main

// Purpose: the user and team a served run counts against in the scheduler limits (D-1044).
// Inputs:  the audit label the user API stored as the submitter of a run, and the run's project.
// Outputs: the principal id (user) and the owner of the project (team).
// Constraints: a run nobody submitted through the user API (a webhook, a provider delivery) has no user,
//          and its team is still the project owner; an empty user and team keep the shared bucket.

import (
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/roles"
)

// submitterIdentity returns the user and team of a run. The team is the owner part of owner/repo: the
// coordinator holds no team table, and the project owner is the organisation the repository belongs to.
func submitterIdentity(submitter, project string) (user, team string) {
	if strings.TrimSpace(submitter) != "" {
		user = roles.PrincipalOf(submitter)
	}
	if owner, _, ok := strings.Cut(project, "/"); ok {
		team = owner
	}
	return user, team
}
