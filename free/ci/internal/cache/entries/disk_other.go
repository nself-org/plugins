//go:build !unix

package entries

// Purpose: platforms without statfs report errNoStatfs and the free-space floor
// check is off there. Inputs: a path. Outputs: errNoStatfs.

func statfs(string) (int64, int64, error) { return 0, 0, errNoStatfs }
