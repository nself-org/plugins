//go:build !unix

package entries

import "errors"

func statfs(string) (int64, int64, error) {
	return 0, 0, errors.New("entries: free-space check unsupported on this platform")
}
