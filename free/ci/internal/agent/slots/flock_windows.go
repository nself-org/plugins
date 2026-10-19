//go:build windows

package slots

import "os"

// lockFile is a stub: windows builds exist (P7-PLUG-69) but the agent does not
// run there, so the lock call reports E654 os_unsupported (Epic D35).
func lockFile(*os.File) error { return &Error{Reason: "os_unsupported"} }

func unlockFile(*os.File) {}
