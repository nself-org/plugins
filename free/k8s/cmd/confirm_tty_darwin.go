// Purpose: the macOS read-termios ioctl request used by isTerminal.
package main

import "syscall"

const ioctlReadTermios = syscall.TIOCGETA
