package main

// Purpose: link the GitLab forge-runner launcher (P7-HOST-07) into the binary so its init() registers the
// demand handler for source gitlab, the gitlab-runner-token minter and the policy keys
// providers.gitlab.runner.tags and providers.gitlab.runner.max_pending. The run finalizer that deregisters a
// run's runners and the orphan sweep are the launcher's, registered once by providers_github_demand.go.
// Dormant unless compat.V15(). The coordinator start must call demand.Bind (the one shared ledger),
// runners.UsePolicy and runtime.UsePolicy before the first demand (P7-NODE-20 wiring); until then the
// handler answers E704 and the demand is offered again.

import _ "github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
