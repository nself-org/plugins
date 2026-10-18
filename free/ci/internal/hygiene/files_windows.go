//go:build windows

package hygiene

func ownedFiles([]string, int, string) []string { return nil }
