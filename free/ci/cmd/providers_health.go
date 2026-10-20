package main

// Purpose: link the provider health breaker (P7-HOST-11) into the binary so its init() registers the
// providers.HealthOf function and the default REST observer. Both are dormant unless compat.V15().

import _ "github.com/nself-org/plugins/free/ci/internal/providers/health"
