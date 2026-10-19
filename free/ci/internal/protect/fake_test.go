package protect

import (
	"context"
	"os"
	"strings"
	"sync"
)

// fakeGH is a recorded `gh` for tests: replies are keyed by a substring of the
// joined argv, first match wins, and every call is recorded.
type fakeGH struct {
	mu      sync.Mutex
	replies []reply
	calls   []string
	bodies  []string // content of every --input file at call time
}

type reply struct {
	match string
	out   string
	exit  int
	then  *reply // when set, the next identical call gets this instead
}

func (f *fakeGH) run(_ context.Context, argv ...string) (string, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	line := strings.Join(argv, " ")
	f.calls = append(f.calls, line)
	for i, a := range argv {
		if a == "--input" && i+1 < len(argv) {
			if b, err := os.ReadFile(argv[i+1]); err == nil {
				f.bodies = append(f.bodies, string(b))
			}
		}
	}
	for i := range f.replies {
		r := &f.replies[i]
		if strings.Contains(line, r.match) {
			out, exit := r.out, r.exit
			if r.then != nil {
				*r = *r.then
			}
			return out, exit, nil
		}
	}
	return "gh: no recorded reply for " + line, 1, nil
}

func (f *fakeGH) client() Client { return Client{Run: f.run} }

func (f *fakeGH) puts() int {
	n := 0
	for _, c := range f.calls {
		if strings.Contains(c, "--method PUT") {
			n++
		}
	}
	return n
}
