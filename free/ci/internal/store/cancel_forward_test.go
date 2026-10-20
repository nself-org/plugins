package store

// A cancel intent consumed by the transition that cancels a superseded attempt is handed to
// the named successor, so an accepted run cancel is not lost (D-1191).

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// parkAttempt walks attempt id through leased, lost and retryable back to queued, the
// state a retried attempt waits in with its retry authorized.
func parkAttempt(t *testing.T, s *Store, id string) int64 {
	t.Helper()
	ctx := context.Background()
	if err := s.testTransition(ctx, id, "queued", "leased", TransitionDetail{Epoch: 0}); err != nil {
		t.Fatal(err)
	}
	epoch := int64(1)
	for _, step := range [][2]model.JobState{{"leased", "lost"}, {"lost", "retryable"}, {"retryable", "queued"}} {
		if err := s.Transition(ctx, id, step[0], step[1], TransitionDetail{Epoch: epoch, FailureClass: "infra"}); err != nil {
			t.Fatal(err)
		}
		epoch++
	}
	return epoch
}

func TestTransitionForwardsTheConsumedCancelIntent(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		intent  bool   // the predecessor has an operator cancel intent
		forward string // ForwardCancelTo
		finish  string // ends the successor first ("" = it stays live)
		want    bool   // the successor ends with an intent
	}{
		"intent forwarded to a live successor": {intent: true, forward: "b", want: true},
		"no intent, nothing to forward":        {forward: "b"},
		"no target named":                      {intent: true},
		"unknown target is not an error":       {intent: true, forward: "missing"},
		"finished successor needs none":        {intent: true, forward: "b", finish: "cancelled"},
	} {
		t.Run(name, func(t *testing.T) {
			s := testStore(t)
			seed(t, s, "a")
			epoch := parkAttempt(t, s, "a")
			if err := s.CreateAttempt(ctx, AttemptRow{ID: "b", JobID: "ja", N: 2}, ""); err != nil {
				t.Fatal(err)
			}
			if tc.intent {
				if err := s.RequestCancel(ctx, "a", "operator stop", epoch); err != nil {
					t.Fatal(err)
				}
			}
			if tc.finish != "" {
				if err := s.testTransition(ctx, "b", "queued", "cancelled", TransitionDetail{Epoch: 0}); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Transition(ctx, "a", "queued", "cancelled", TransitionDetail{Epoch: epoch, ForwardCancelTo: tc.forward}); err != nil {
				t.Fatal(err)
			}
			if has, _ := s.CancelIntent(ctx, "a"); has {
				t.Fatal("the predecessor must not keep its consumed intent")
			}
			has, err := s.CancelIntent(ctx, "b")
			if err != nil || has != tc.want {
				t.Fatalf("successor intent = %v (err %v), want %v", has, err, tc.want)
			}
		})
	}
}
