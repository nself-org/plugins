//go:build linux

package exec

import "syscall"

// setDataLimit sets RLIMIT_DATA, soft and hard, as `ulimit -d` does (the memory cap that counts writable
// private memory; RLIMIT_AS is never used, it kills the Go and V8 runtimes at start).
func setDataLimit(bytes uint64) error {
	return syscall.Setrlimit(syscall.RLIMIT_DATA, &syscall.Rlimit{Cur: bytes, Max: bytes})
}
