package main

// Purpose: link the GitHub trigger source (P7-HOST-05) into the binary so its init() registers the key
// providers.github.webhook_secrets, the TriggerSource "github" and the provider "github" that serves
// trust-fact refetch. Dormant unless compat.V15() (the runtime ingress adapter routes to it only then).

import _ "github.com/nself-org/plugins/free/ci/internal/providers/github/webhook"
