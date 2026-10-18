package cli

// Purpose: the verbatim help text of `nself ci eval` and `nself ci eval gate`
//   as captured from core (testdata/eval-parity/*-help.stdout).
// Inputs:  none. Outputs: the embedded text.
// Constraints: the text keeps core's "nself ci eval" wording until core is
//   deleted at P7-SHIP-09; TestEvalParity compares it to the goldens.
// SPORT: PLUGINS-NSELF-EVAL-GATE-CLI-001

import _ "embed"

//go:embed help/eval.txt
var evalHelp string

//go:embed help/gate.txt
var gateHelp string
