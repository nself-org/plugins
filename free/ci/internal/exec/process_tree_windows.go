//go:build windows

package exec

type processTree struct{}

func trackDescendants(int, string) *processTree { return &processTree{} }
func (*processTree) snapshot()                  {}
func (*processTree) signalTerminate()           {}
func (*processTree) signalKill()                {}
func (*processTree) anyAlive() bool             { return false }
func (*processTree) stop()                      {}
