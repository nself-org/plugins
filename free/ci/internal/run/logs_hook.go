package run

// Purpose: the seam that stores each attempt's output (P7-CI-39). The executor
// redacts every chunk before it reaches the run's output callback; that
// callback hands the redacted bytes to the attempt's AttemptLog.
// Inputs:  the registered LogHook, the session and the attempt.
// Outputs: an AttemptLog per attempt, closed when the executor returns.
// Constraints: storing a log never fails or slows a job beyond the hook's own
// work: an open error, a panic or a write error leaves the job running with
// no stored log (the hook has no channel to report it, so a missing log can mean
// a storage failure; `ci logs` says so); the hook only
// ever sees redacted bytes, because the funnel's Redactor runs first.

import "context"

// AttemptLog receives the redacted output of one attempt.
type AttemptLog interface {
	Write(p []byte) (int, error)
	Close() error
}

// LogHook opens the log of an attempt. It may return nil (no log). It runs
// once per attempt, on the attempt's goroutine, before the job starts; the
// first call of a run is its "run start" (the place retention runs).
type LogHook interface {
	OpenAttempt(ctx context.Context, s *Session, a AttemptInfo) AttemptLog
}

// StateDir is the run's state directory: Options.Home, else NSELF_CI_HOME,
// else ~/.nself/ci ("" when no home directory is known).
func (s *Session) StateDir() string { return ciHome(s.Home) }

// openLog asks the hook for the attempt's log; a panic yields no log.
func (rs *runState) openLog(ctx context.Context, info AttemptInfo) (lg AttemptLog) {
	if rs.hooks.Logs == nil {
		return nil
	}
	_ = guard("log hook", func() error { lg = rs.hooks.Logs.OpenAttempt(ctx, rs.sess, info); return nil })
	return lg
}

func writeLog(lg AttemptLog, b []byte) {
	if lg != nil {
		_ = guard("log write", func() error { _, err := lg.Write(b); return err })
	}
}

func closeLog(lg AttemptLog) {
	if lg != nil {
		_ = guard("log close", func() error { return lg.Close() })
	}
}
