package evidence

// Purpose: reuse immutable object bytes while detectors and digesters reread a tree.
// Inputs: one listed blob id and its size. Outputs: its original git bytes.
// Constraints: scope is one TreeFS; failures are retried, and readers own their
// offsets. The mutex also coalesces simultaneous opens of the same object.
// SPORT: no checkout data or mutable ref is cached.

import (
	"context"
	"fmt"
	"time"
)

func (t *treeFS) readBlob(b blob) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if out, ok := t.data[b.oid]; ok {
		return out, nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.ctx), 60*time.Second)
	defer cancel()
	out, ok := git(ctx, t.c, t.root, "cat-file", "blob", b.oid)
	if !ok || int64(len(out)) != b.size {
		return "", fmt.Errorf("unreadable blob %s", short(b.oid))
	}
	if t.data == nil {
		t.data = make(map[string]string)
	}
	t.data[b.oid] = out
	return out, nil
}
