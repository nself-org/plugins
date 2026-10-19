package agent

// Purpose: the plain-test entry points of the acceptance scenarios (TestAgentPull runs the same ones under the
//          integration tag with the container scenarios added).
// Inputs:  scenarios_test.go. Constraints: none beyond the scenarios'.

import (
	"testing"
	"time"
)

func TestPullPartitionResumesWithoutDuplicates(t *testing.T) {
	scenarioPartition(t, 1500*time.Millisecond)
}
func TestPullCoordinatorKillResumesWithoutDuplicates(t *testing.T) { scenarioRestart(t) }
