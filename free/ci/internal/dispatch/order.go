package dispatch

// Purpose: the queue order the dispatcher hands to sched.Place (Epic D7).
// Inputs:  the queued jobs. Outputs: the same jobs in placement order.
// Constraints: the default is age FIFO (enqueue time, then arrival number);
// P7-SCHED-06 replaces it with start-time fair queuing through Config.Orderer.
// An Orderer must be deterministic and return exactly the jobs it was given.

import (
	"sort"
	"time"
)

// Queued is a job with its place in the queue.
type Queued struct {
	Job        Job
	EnqueuedAt time.Time
	Seq        uint64
}

// Orderer sets the order jobs are placed and admitted in.
type Orderer interface {
	Order([]Queued) []Queued
}

// FIFO orders by age, oldest first.
type FIFO struct{}

// Order implements Orderer; the input is not modified.
func (FIFO) Order(q []Queued) []Queued {
	out := append([]Queued(nil), q...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].EnqueuedAt.Equal(out[j].EnqueuedAt) {
			return out[i].EnqueuedAt.Before(out[j].EnqueuedAt)
		}
		return out[i].Seq < out[j].Seq
	})
	return out
}
