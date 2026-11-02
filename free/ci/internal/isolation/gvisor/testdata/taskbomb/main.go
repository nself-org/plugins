//go:build linux && (amd64 || arm64)

// This bounded fixture creates tasks sharing one address space. Only the original
// Go runtime threads execute Go; cloned tasks remain in an assembly futex wait.
package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"syscall"
	"unsafe"
)

const taskLimit = 512
const stackSize = 16384

// cloneTask returns a Linux clone result. The child never returns to Go.
func cloneTask(stack uintptr) int64

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FAIL: "+format+"\n", args...)
	os.Exit(1)
}

// countTasks uses only raw syscalls after saturation, so Go cannot try to
// start another runtime thread while entering a blocking syscall.
//
//go:nosplit
func countTasks(fd int, buf []byte) (int, syscall.Errno) {
	count := 0
	for {
		n, _, errno := syscall.RawSyscall(syscall.SYS_GETDENTS64, uintptr(fd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if errno != 0 || n == 0 {
			return count, errno
		}
		for offset := 0; offset < int(n); {
			if int(n)-offset < 20 {
				return count, syscall.EIO
			}
			size := int(buf[offset+16]) | int(buf[offset+17])<<8
			if size < 20 || size > int(n)-offset {
				return count, syscall.EIO
			}
			name := buf[offset+19 : offset+size]
			dot := name[0] == '.' && ((len(name) > 1 && name[1] == 0) || (len(name) > 2 && name[1] == '.' && name[2] == 0))
			if !dot {
				count++
			}
			offset += size
		}
	}
}

// prepare allocates and opens everything before the guest task table is full.
func prepare() (stack []byte, fd int, buf, report []byte) {
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	debug.SetGCPercent(-1)
	var limit syscall.Rlimit
	// Linux RLIMIT_NPROC is 6 on both supported architectures.
	if err := syscall.Getrlimit(6, &limit); err != nil {
		fail("getrlimit: %v", err)
	}
	if limit.Cur != taskLimit || limit.Max != taskLimit {
		fail("nproc limit: soft=%d hard=%d", limit.Cur, limit.Max)
	}
	stack, err := syscall.Mmap(-1, 0, taskLimit*stackSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil {
		fail("child stacks: %v", err)
	}
	fd, err = syscall.Open("/proc/self/task", syscall.O_RDONLY|syscall.O_DIRECTORY, 0)
	if err != nil {
		fail("open tasks: %v", err)
	}
	buf = make([]byte, 32768)
	report = []byte("nself-ci: task limit reached: limit=512 tasks=512 errno=EAGAIN\n")
	// Suppress async Go preemption on the locked parent M throughout saturation.
	// The children inherit this mask; every successful path exits the whole group.
	mask := ^uint64(0)
	if _, _, errno := syscall.RawSyscall6(syscall.SYS_RT_SIGPROCMASK, 2, uintptr(unsafe.Pointer(&mask)), 0, 8, 0, 0); errno != 0 {
		fail("block signals: %v", errno)
	}
	return stack, fd, buf, report
}

func main() {
	stack, fd, buf, report := prepare()
	for attempt := 0; attempt < taskLimit; attempt++ {
		result := cloneTask(uintptr(unsafe.Pointer(&stack[(attempt+1)*stackSize-16])))
		runtime.KeepAlive(stack)
		if result > 0 {
			continue
		}
		if result != -int64(syscall.EAGAIN) {
			fail("clone returned %d, want EAGAIN", result)
		}
		tasks, errno := countTasks(fd, buf)
		if errno != 0 {
			fail("count tasks: %v", errno)
		}
		if tasks != taskLimit {
			fail("clone EAGAIN with %d tasks, want %d", tasks, taskLimit)
		}
		n, _, errno := syscall.RawSyscall(syscall.SYS_WRITE, 1, uintptr(unsafe.Pointer(&report[0])), uintptr(len(report)))
		if errno != 0 || int(n) != len(report) {
			fail("write report: bytes=%d errno=%v", n, errno)
		}
		syscall.RawSyscall(syscall.SYS_EXIT_GROUP, 2, 0, 0)
		for { // exit_group does not return; never re-enter the runtime if it does.
		}
	}
	fail("all %d bounded clone attempts succeeded", taskLimit)
}
