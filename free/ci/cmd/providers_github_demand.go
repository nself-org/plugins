package main

// Purpose: link the GitHub forge-runner launcher (P7-HOST-19) into the binary so its init() registers the
// demand handler for source github, the github-jitconfig minter, the transport preparer that mints the
// credential after admission, the run finalizer and the orphan sweep Reconciler, and the policy key
// providers.github.jit.max_pending. Dormant unless compat.V15(). The coordinator start must call
// demand.Bind(store) and demand.UsePolicy before the first demand (P7-NODE-20 wiring); until then the
// handler answers E704 and the demand is offered again.

import _ "github.com/nself-org/plugins/free/ci/internal/providers/github/demand"
