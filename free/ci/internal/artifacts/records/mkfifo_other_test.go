//go:build !unix

package records_test

const fifoSupported = false

func mkfifo(string) error { return nil }
