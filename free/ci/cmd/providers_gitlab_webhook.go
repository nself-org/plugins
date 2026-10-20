package main

// Purpose: link the GitLab trigger source (P7-HOST-21) into the binary so its init() registers the keys
// providers.gitlab.webhook_signing_secrets and providers.gitlab.webhook_tokens, the TriggerSource
// "gitlab" and the trust-fact refetch of the "gitlab" provider (through the pipeline provider's
// UseTrustFacts). Dormant unless compat.V15() (the runtime ingress adapter routes to it only then).

import _ "github.com/nself-org/plugins/free/ci/internal/providers/gitlab/webhook"
