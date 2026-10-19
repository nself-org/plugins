//go:build !unix

package checkout

import "os"

func tryLock(*os.File) bool { return true }
