package main

// Purpose: link the coordinator schedule source (P7-HOST-20) into the binary so its init() registers
// the policy key triggers.schedules and the "schedule" Reconciler. Dormant unless compat.V15().

import _ "github.com/nself-org/plugins/free/ci/internal/triggers/schedule"
