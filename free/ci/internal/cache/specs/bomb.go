package specs

// Purpose: stop a decompression bomb while restoring a cache archive (R-SEC-29).
// Inputs:  the compressed byte counter under the gzip reader and the limits.
// Outputs: guardedEntry (wraps one entry's data), bombGuard.finish.
// Constraints: the ratio (output : compressed) is judged exactly, per entry
// when its data is written and over the whole archive at its end, with an 8 KiB
// allowance for the bytes the decompressor reads ahead; judging a prefix would
// refuse legitimate files that start with a run of zeros. While writing, only a
// large-output resource guard runs (streamFloor, 64 KiB allowance), so a bomb
// costs at most streamFloor bytes of disk before it stops. The sanitiser's own
// ratio check never fires (it is given Packed = Size at the header).

import (
	"io"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

const (
	ratioSlack  = 64 << 10  // read-ahead forgiven by the streaming guard
	finalSlack  = 8 << 10   // read-ahead forgiven by the exact checks (flate buffers 4 KiB)
	streamFloor = 256 << 20 // output before the streaming guard looks at the ratio
)

type bombGuard struct {
	cr    *countReader
	lim   authz.Limits
	total int64
}

type guardedEntry struct {
	g     *bombGuard
	r     io.Reader
	name  string
	start int64
	n     int64
}

func (g *bombGuard) entry(r io.Reader, name string) *guardedEntry {
	return &guardedEntry{g: g, r: r, name: name, start: g.cr.n}
}

func (e *guardedEntry) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	e.n += int64(n)
	e.g.total += int64(n)
	g := e.g
	if (e.n > streamFloor && e.n > g.lim.MaxRatio*(g.cr.n-e.start+ratioSlack)) || (g.total > streamFloor && g.total > g.lim.MaxRatio*(g.cr.n+ratioSlack)) {
		return n, &authz.PathError{Reason: authz.SizeRatio, Name: e.name}
	}
	return n, err
}

// done judges the entry's exact ratio once all its bytes were written.
func (e *guardedEntry) done() error {
	if g := e.g; e.n > g.lim.RatioFloor && e.n > g.lim.MaxRatio*(g.cr.n-e.start+finalSlack) {
		return &authz.PathError{Reason: authz.SizeRatio, Name: e.name}
	}
	return nil
}

// finish judges the whole archive once every entry was written.
func (g *bombGuard) finish() error {
	if g.total > g.lim.RatioFloor && g.total > g.lim.MaxRatio*(g.cr.n+finalSlack) {
		return &authz.PathError{Reason: authz.SizeRatio, Name: "(archive)"}
	}
	return nil
}
