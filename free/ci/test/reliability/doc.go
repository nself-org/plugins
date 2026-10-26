// Package reliability holds the lost-runner fault suite (scenario 9 and the
// reliability items of P7-NODE-11).
//
// Two layers run the same assertions:
//
//   - The sim layer (sim_*_test.go and the TestFault* tests) drives the real
//     store, lease engine, agent fence, drain hand-back, placement and log
//     writer with injected wall and monotonic clocks. Freezes, partitions,
//     suspends, clock jumps, crashes and coordinator outages are clock and
//     carrier events, so no test sleeps. It runs in every `go test`.
//   - The Docker layer (docker_*_test.go) applies the same faults to real
//     sshd containers through sdk/go/simharness: docker pause, a network
//     partition, and a container restart. It runs only with INTEGRATION=1 on
//     a Linux Docker Engine; without it simharness skips the test.
//
// Every test asserts store state, the evidence attempts and their
// failure_class, process timestamps, and that the fixture job's side-effect
// file is written at most once.
package reliability
