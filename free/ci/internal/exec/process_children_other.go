//go:build !linux && !windows

package exec

// quickTable has no native shortcut here: the caller reads the full table.
func quickTable(self, root int, seen []int, attempt string) (map[int]int, bool) {
	return nil, false
}
