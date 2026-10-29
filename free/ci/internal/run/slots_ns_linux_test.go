//go:build linux

package run

import (
	"fmt"
	"os"
	"sync/atomic"
	"testing"
)

var slotNSSeq atomic.Int64

// privateSlots points the Linux host slot lease at a namespace only this test uses, so slots it leaves held
// cannot starve other tests or collide with a real coordinator on the same host.
func privateSlots(t *testing.T) {
	t.Helper()
	old := hostSlotDir
	hostSlotDir = fmt.Sprintf("@nself-ci/test/%d/%d", os.Getpid(), slotNSSeq.Add(1))
	t.Cleanup(func() { hostSlotDir = old })
}
