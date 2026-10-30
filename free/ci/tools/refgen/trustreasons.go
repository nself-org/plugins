package main

// Purpose: the trust reasons table of the Trust, Security and Privacy guide.
// Inputs:  trust.ReasonMeaning, the one map of reason code to meaning.
// Outputs: a Markdown table sorted by reason code.
// Constraints: the table is generated only; the guide holds it between
// <!-- BEGIN GENERATED:trust-reasons --> markers.

import (
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// trustReasonsTable renders every trust reason code and its meaning.
func trustReasonsTable() string {
	keys := make([]string, 0, len(trust.ReasonMeaning))
	for r := range trust.ReasonMeaning {
		keys = append(keys, string(r))
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("| Reason | Meaning |\n|---|---|\n")
	for _, k := range keys {
		b.WriteString("| `" + k + "` | " + cell(trust.ReasonMeaning[trust.Reason(k)]) + " |\n")
	}
	return b.String()
}
