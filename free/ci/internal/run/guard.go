package run

// guard calls a hook and turns a panic into an E627 error, so one bad hook
// cannot kill the coordinator and orphan the other jobs.
func guard(name string, f func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fail(CodeOptions, "%s panicked: %v", name, r)
		}
	}()
	return f()
}
