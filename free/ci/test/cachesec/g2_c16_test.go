//go:build cachesec

package cachesec

// Purpose: TM-16 cases C16-ART-01..03: a registered secret value smuggled into an artifact (raw, base64,
// in a file name) is refused by the collector (E712 secret_match) and nothing is stored or staged.
// Production appends no audit row for the scan verdict: the rows say `audit: none, gap: D-1450` and
// each case asserts that no audit row appeared.
// Inputs: g2World with a redact.Redactor. Outputs: registered cases of matrix.d/g2.yaml.
// Constraints: synthetic value only; honest control (a clean file in the same workdir) first.

import (
	"encoding/base64"
	"strings"
	"testing"
)

const g2Secret = "SYNTHETIC_TOKEN_aaaa1111bbbb2222"

func g2SecretCase(setup func(g *g2World) string) func(*testing.T) Observed {
	return func(t *testing.T) Observed {
		g := newG2World(t)
		if err := g.Red.Register([]byte(g2Secret)); err != nil {
			t.Fatal(err)
		}
		g.Write("clean.txt", "nothing to see")
		if err := g2Collect(g, "clean.txt"); err != nil {
			t.Fatalf("control: %v", err)
		}
		path := setup(g)
		g.Mark()
		err := g2Collect(g, path)
		got := refusal(t, err, func(err error) string { return g2RecordsReason(t, err) })
		if strings.Contains(err.Error(), g2Secret) {
			t.Fatal("the refusal leaks the registered value")
		}
		got.Audit, got.Landed = g.AuditSince(), g.LandedSince()
		return got
	}
}

func init() {
	register("C16-ART-01", g2SecretCase(func(g *g2World) string {
		g.Write("raw.txt", "prefix "+g2Secret+" suffix")
		return "raw.txt"
	}))
	register("C16-ART-02", g2SecretCase(func(g *g2World) string {
		g.Write("b64.txt", "prefix "+base64.StdEncoding.EncodeToString([]byte(g2Secret))+" suffix")
		return "b64.txt"
	}))
	register("C16-ART-03", g2SecretCase(func(g *g2World) string {
		g.Write("d/"+g2Secret+".txt", "innocent body")
		return "d"
	}))
}
