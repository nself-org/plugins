//go:build !unix

package sbom

import "os"

// ownedByUs: file ownership is not checked off unix (the ACL model differs).
func ownedByUs(os.FileInfo) bool { return true }
