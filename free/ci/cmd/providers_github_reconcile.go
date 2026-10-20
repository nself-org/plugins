package main

// Purpose: link the GitHub reconciliation sweep and the poll-mode demand poll (P7-HOST-17) into the
// binary so its init() registers the keys providers.github.ingress, providers.github.poll_interval and
// providers.github.hook_url and the Reconcilers github-reconcile and github-demand-poll. Dormant unless
// compat.V15() (both Reconcilers return at once otherwise).

import _ "github.com/nself-org/plugins/free/ci/internal/providers/github/reconcile"
