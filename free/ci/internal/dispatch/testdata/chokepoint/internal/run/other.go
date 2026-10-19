package run

// Planted: LocalAdmit is only for CreateAttempt and the dispatch hook.
func Back() error { return LocalAdmit() }

// Planted: a function value of LocalAdmit.
func Value() func() error { return LocalAdmit }

// A METHOD named CreateAttempt is not run.CreateAttempt.
func (T) CreateAttempt(s *store.Store) error { return s.AdmitAndLease(ctx, "host", demand) }

// Wrappers of Transition: a "leased" target, a variable target, a method value.
func Settle(rs *runState) { rs.settle(id, "queued", "leased", "") }

func Forward(rs *runState, to model.JobState) { rs.trans(id, "queued", to, "") }

func ValueTrans(rs *runState) any { f := rs.trans; return f }

// Method expressions shift the arguments and hide the target.
func ExprTrans(rs *runState) { (*runState).trans(rs, id, "queued", "leased", "") }

func ExprTransition(s *store.Store) error {
	return (*store.Store).Transition(s, ctx, "a", "queued", "leased", detail)
}
