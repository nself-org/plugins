// Purpose: the Linux read-termios ioctl request used by isTerminal.
package main

import "syscall"

const ioctlReadTermios = syscall.TCGETS
