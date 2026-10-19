package specs

// Purpose: delete a restored cache tree on several goroutines (D-1008 round 3).
// A Go build cache is a thousand files in 256 directories; unlinking them one by
// one was about 50 ms on the end of every job that restored one.
// Inputs:  a pinned root and a directory below it (already owner-writable).
// Outputs: the directory gone, like os.Root.RemoveAll.
// Constraints: every call goes through the pinned root, so nothing leaves it; the
// fan-out starts at the first directory with fanMin or more subdirectories, and a
// narrower chain is walked in place; errors are ignored like the callers' own.

import (
	"os"
	"path"
	"sync"
)

const (
	fanMin     = 4
	fanWorkers = 8
)

// afterRemoveStart is a test seam: called as a removal goroutine starts on a subtree.
var afterRemoveStart func()

// removeAll deletes rel below root.
func removeAll(root *os.Root, rel string) error {
	if info, err := root.Lstat(rel); err != nil || !info.IsDir() {
		return root.RemoveAll(rel)
	}
	fanRemove(root, rel)
	return root.RemoveAll(rel) // whatever is left, and the directory itself
}

func fanRemove(root *os.Root, rel string) {
	f, err := root.Open(rel)
	if err != nil {
		return
	}
	ents, err := f.ReadDir(-1)
	_ = f.Close()
	if err != nil {
		return
	}
	var subs []string
	for _, e := range ents {
		if e.IsDir() {
			subs = append(subs, path.Join(rel, e.Name()))
		}
	}
	if len(subs) < fanMin {
		for _, s := range subs {
			fanRemove(root, s)
		}
		return
	}
	var wg sync.WaitGroup
	next := make(chan string)
	for range min(fanWorkers, len(subs)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range next {
				if afterRemoveStart != nil {
					afterRemoveStart()
				}
				_ = root.RemoveAll(s)
			}
		}()
	}
	for _, s := range subs {
		next <- s
	}
	close(next)
	wg.Wait()
}
