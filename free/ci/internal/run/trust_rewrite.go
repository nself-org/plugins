package run

import (
	"context"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

// verifyRewrites checks patch provenance independently of the reflog's creation kind.
// git cherry compares stable patch IDs with all commits reachable from each fetched ref,
// including pull refs and each FETCH_HEAD entry. Only a positive, unique patch result
// for every history can raise trust. A failed comparison leaves verification false.
func (h SourceTrust) verifyRewrites(ctx context.Context, root string, g *source.GitFacts, refs []string) {
	git := h.git(ctx, root)
	g.RewriteVerified = make(map[string]bool)
	for _, c := range g.OffDefault {
		if !source.Rewritten(g.ReflogCreated[c.SHA]) {
			continue
		}
		clean := true
		for _, ref := range refs {
			out, exit := git("cherry", ref, c.SHA)
			unique := false
			if exit == 0 {
				for _, line := range lines(out) {
					fields := strings.Fields(line)
					if len(fields) == 2 && fields[0] == "+" && fields[1] == c.SHA {
						unique = true
					}
				}
			}
			if !unique {
				clean = false
				break
			}
		}
		g.RewriteVerified[c.SHA] = clean
	}
}
