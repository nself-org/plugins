package main

// Purpose: the GitLab half of `nself ci providers test` (P7-HOST-12, Epic D14, review F14): the token
// in providers.gitlab.token_env asks GitLab for its own record (GET /personal_access_tokens/self: name,
// scopes, active, expiry) and for the role it holds on the project (GET /projects/:id permissions),
// and the token in providers.gitlab.runner_token_env is checked for the create_runner scope.
// Inputs:  the operator policy (token variables, API URL), --repo group/project.
// Outputs: probeRow values; no credential value, only names, scopes and the role.
// Constraints: read only; the role is the higher of the project and group access levels GitLab
//          reports; a missing --repo skips the role row (GitLab has no origin-remote shortcut here).

import (
	"context"
	"fmt"
	"slices"
	"strings"

	pdoctor "github.com/nself-org/plugins/free/ci/internal/providers/doctor"
)

type glTokenSelf struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	Active    bool     `json:"active"`
	Revoked   bool     `json:"revoked"`
	ExpiresAt string   `json:"expires_at"`
}

// glRoles are the GitLab access levels by number.
var glRoles = map[int]string{0: "No access", 5: "Minimal access", 10: "Guest", 15: "Planner", 20: "Reporter", 30: "Developer", 40: "Maintainer", 50: "Owner"}

func glRoleName(level int) string {
	if n, ok := glRoles[level]; ok {
		return n
	}
	return fmt.Sprintf("level %d", level)
}

// glSelf asks the token for its own record. credRow names the row for the credential itself.
func glSelf(ctx context.Context, env providersEnv, cfg pdoctor.Config, runner bool, label string) ([]probeRow, glTokenSelf) {
	cred := pdoctor.GitLabCredential(cfg, env.Getenv, runner)
	if !cred.Present {
		detail := "no GitLab token resolves (" + cred.Source + ")"
		if runner && cred.Source == "none" {
			return []probeRow{{label, probeSkip, "providers.gitlab.runner_token_env is not set (runner registration is off)"}}, glTokenSelf{}
		}
		return []probeRow{{label, probeFail, detail}}, glTokenSelf{}
	}
	c, err := env.NewClient(cfg, "gitlab", runner)
	if err != nil {
		return []probeRow{{label, probeFail, oneLineMsg(err)}}, glTokenSelf{}
	}
	var self glTokenSelf
	if _, err = c.Do(ctx, "GET", "/personal_access_tokens/self", nil, nil, &self); err != nil {
		return []probeRow{{label, probeFail, cred.Source + ": " + providerFailure(err)}}, glTokenSelf{}
	}
	return nil, self
}

func probeGitLab(ctx context.Context, env providersEnv, cfg pdoctor.Config, project string) []probeRow {
	var rows []probeRow
	failRows, self := glSelf(ctx, env, cfg, false, "token")
	if failRows != nil {
		rows = append(rows, failRows...)
	} else {
		switch {
		case self.Revoked || !self.Active:
			rows = append(rows, probeRow{"token", probeFail, "token " + self.Name + " is not active"})
		default:
			detail := "token " + self.Name + ", scopes [" + strings.Join(self.Scopes, ", ") + "]"
			if self.ExpiresAt != "" {
				detail += ", expires " + self.ExpiresAt
			}
			rows = append(rows, probeRow{"token", probeOK, detail})
		}
		if slices.Contains(self.Scopes, "api") {
			rows = append(rows, probeRow{"token scope api", probeOK, "the token can create pipelines"})
		} else {
			rows = append(rows, probeRow{"token scope api", probeFail, "the token lacks the api scope"})
		}
		if project != "" {
			rows = append(rows, probeGitLabRole(ctx, env, cfg, project))
		} else {
			rows = append(rows, probeRow{"project role", probeSkip, "pass --repo group/project"})
		}
	}
	runRows, rself := glSelf(ctx, env, cfg, true, "runner token")
	switch {
	case runRows != nil:
		rows = append(rows, runRows...)
	case rself.Revoked || !rself.Active:
		rows = append(rows, probeRow{"runner token", probeFail, "token " + rself.Name + " is not active"})
	case !slices.Contains(rself.Scopes, "create_runner"):
		rows = append(rows, probeRow{"runner token", probeFail, "scopes [" + strings.Join(rself.Scopes, ", ") + "] lack create_runner"})
	default:
		rows = append(rows, probeRow{"runner token", probeOK, "scopes [" + strings.Join(rself.Scopes, ", ") + "]"})
	}
	if project != "" {
		signing, token := cfg.GitLabSecretEnvs(project)
		rows = append(rows, probeSecretEnv("webhook signing secret", signing, env), probeSecretEnv("webhook token", token, env))
	}
	return rows
}

func probeGitLabRole(ctx context.Context, env providersEnv, cfg pdoctor.Config, project string) probeRow {
	c, err := env.NewClient(cfg, "gitlab", false)
	if err != nil {
		return probeRow{"project role", probeFail, oneLineMsg(err)}
	}
	var info struct {
		Permissions struct {
			Project *struct {
				Level int `json:"access_level"`
			} `json:"project_access"`
			Group *struct {
				Level int `json:"access_level"`
			} `json:"group_access"`
		} `json:"permissions"`
	}
	if _, err = c.Do(ctx, "GET", "/projects/{id}", map[string]string{"id": project}, nil, &info); err != nil {
		return probeRow{"project role", probeFail, project + ": " + providerFailure(err)}
	}
	level := 0
	for _, l := range []*struct {
		Level int `json:"access_level"`
	}{info.Permissions.Project, info.Permissions.Group} {
		if l != nil {
			level = max(level, l.Level)
		}
	}
	role := glRoleName(level)
	if level < 30 {
		return probeRow{"project role", probeFail, "the token holds the " + role + " role on " + project + "; creating pipelines needs Developer or higher"}
	}
	return probeRow{"project role", probeOK, "the token holds the " + role + " role on " + project}
}
