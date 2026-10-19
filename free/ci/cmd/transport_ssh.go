package main

// Purpose: registers the `ssh` transport driver (carrier A, push) in the
// `nself ci` binary so transport.Lookup("ssh") finds it; P7-SCHED-04 delivers
// leases through that lookup and leaves a lease of an unknown kind queued.
// Inputs:  none. Outputs: the driver registered by the package's init.

import _ "github.com/nself-org/plugins/free/ci/internal/transport/ssh"
