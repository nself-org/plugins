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

func TestPullWireRefusalsAreE661AndCancelTheLease(t *testing.T) { scenarioWireRefusals(t) }

func TestPullCancelDuringPartitionReachesTheAgentOnReconnect(t *testing.T) {
	scenarioCancelDuringPartition(t, false)
}

func TestPullCancelAfterTheRunGaveUpReachesTheAgentThroughTheEpoch(t *testing.T) {
	scenarioCancelDuringPartition(t, true)
}

func TestPullRevokeWhilePollingClosesTheStreamAndRefusesTheHandshake(t *testing.T) {
	scenarioRevokeWhilePolling(t)
}
func TestPullRefusesAServerLeafOfNodeOrUserKind(t *testing.T) { scenarioServerLeafKinds(t) }
