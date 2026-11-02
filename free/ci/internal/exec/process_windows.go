//go:build windows

package exec

import osexec "os/exec"

func prepareProcess(_ *osexec.Cmd) error      { return coded("E610", "os_unsupported") }
func terminateGroup(_ int) error              { return coded("E610", "os_unsupported") }
func killGroup(_ int) error                   { return coded("E610", "os_unsupported") }
func groupAlive(_ int) bool                   { return false }
func killPID(_ int) error                     { return coded("E610", "os_unsupported") }
func isTerminal() bool                        { return false }
func gatedCommand(s JobSpec) ([]string, bool) { return s.Command, false }
func exitSignal(_ *osexec.ExitError) string   { return "" }
