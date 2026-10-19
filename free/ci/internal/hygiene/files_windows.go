//go:build windows

package hygiene

func ownedFiles([]string, int, string) ([]string, bool) { return nil, true }
