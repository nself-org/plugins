package main

// Purpose: link the hosted provider runtime (P7-HOST-16) into the binary so its init() registers the
// providers.* policy keys, the transport driver "provider", the world contributor "providers" and
// the trigger-source ingress adapter. Each of them is dormant unless compat.V15().

import _ "github.com/nself-org/plugins/free/ci/internal/providers/runtime"
