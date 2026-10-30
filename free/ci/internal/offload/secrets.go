package offload

// Purpose: the secret path list and size cap for the dirty-tree snapshot.
// Inputs:  repository-relative paths and file sizes.
// Outputs: whether a path is secret-shaped or too large.
// Constraints: decided on the path and size only, never on file content.

import (
	"path"
	"strings"
)

// MaxFileBytes is the size cap for one file in a snapshot (10 MiB).
const MaxFileBytes = 10 << 20

// SecretPath reports whether the base name of p is on the secret path list:
// .env*, *.pem, *.key, id_*, .npmrc, .netrc and credentials files.
func SecretPath(p string) bool {
	b := strings.ToLower(path.Base(p))
	switch {
	case b == ".env", strings.HasPrefix(b, ".env."):
		return true
	case strings.HasSuffix(b, ".pem"), strings.HasSuffix(b, ".key"):
		return true
	case strings.HasPrefix(b, "id_"):
		return true
	case b == ".npmrc", b == ".netrc", b == ".git-credentials":
		return true
	case b == "credentials", strings.HasPrefix(b, "credentials."):
		return true
	}
	return false
}
