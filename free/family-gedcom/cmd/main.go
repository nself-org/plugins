// family-gedcom plugin entrypoint — GEDCOM 5.5/7.0 to JSON converter.
//
// Purpose: Parse a GEDCOM file and convert individuals and families to the
//          family plugin's expected JSON schema. Outputs JSON to stdout.
//          Designed as a stateless import tool — no DB interaction.
//
// Inputs:  GEDCOM_FILE env — path to GEDCOM file to parse.
// Outputs: JSON array of parsed individuals to stdout; exit 0 on success.
// Constraints: v0.0.1 targets GEDCOM 5.5.1 basics (INDI + FAM records).
// SPORT: PLUGINS-FAMILY-GEDCOM-000
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nself-org/plugins/free/family-gedcom/internal/gedcom"
)

func main() {
	gedcomFile := os.Getenv("GEDCOM_FILE")
	if gedcomFile == "" {
		fmt.Fprintln(os.Stderr, "family-gedcom: GEDCOM_FILE env not set")
		os.Exit(1)
	}

	result, err := gedcom.ParseGEDCOM(gedcomFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "family-gedcom: parse error: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "family-gedcom: json encode error: %v\n", err)
		os.Exit(1)
	}
}
