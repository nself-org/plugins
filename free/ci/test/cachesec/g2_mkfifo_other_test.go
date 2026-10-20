//go:build cachesec && !unix

package cachesec

// Purpose: no FIFO on this platform; the crafted-tree test of group g2 leaves that entry out.

import "errors"

const g2FifoSupported = false

func g2Mkfifo(string) error { return errors.New("no mkfifo here") }
