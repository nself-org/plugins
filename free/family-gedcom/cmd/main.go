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
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Individual represents a parsed GEDCOM INDI record.
type Individual struct {
	ID        string   `json:"id"`
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	Sex       string   `json:"sex"`
	BirthDate string   `json:"birth_date,omitempty"`
	BirthPlace string  `json:"birth_place,omitempty"`
	FamilyIDs []string `json:"family_ids,omitempty"`
}

// ParseResult holds the output of a GEDCOM parse.
type ParseResult struct {
	Individuals []Individual `json:"individuals"`
	FamilyCount int          `json:"family_count"`
}

func main() {
	gedcomFile := os.Getenv("GEDCOM_FILE")
	if gedcomFile == "" {
		fmt.Fprintln(os.Stderr, "family-gedcom: GEDCOM_FILE env not set")
		os.Exit(1)
	}

	result, err := ParseGEDCOM(gedcomFile)
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

// ParseGEDCOM parses a GEDCOM 5.5.1 file and returns individuals and families.
// This is a minimal parser targeting the INDI and FAM record types.
func ParseGEDCOM(path string) (*ParseResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	var individuals []Individual
	var familyCount int

	var current *Individual
	var inBirt bool

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(strings.TrimSpace(line), " ", 3)
		if len(parts) < 2 {
			continue
		}

		level := parts[0]
		tag := parts[1]
		var value string
		if len(parts) == 3 {
			value = parts[2]
		}

		switch {
		case level == "0" && strings.HasPrefix(tag, "@") && len(parts) == 3 && parts[2] == "INDI":
			// Start of a new individual record
			if current != nil {
				individuals = append(individuals, *current)
			}
			current = &Individual{ID: strings.Trim(tag, "@")}
			inBirt = false

		case level == "0" && strings.HasPrefix(tag, "@") && len(parts) == 3 && parts[2] == "FAM":
			// Start of a family record
			if current != nil {
				individuals = append(individuals, *current)
				current = nil
			}
			familyCount++
			inBirt = false

		case level == "0":
			// Other top-level record (HEAD, TRLR, etc.) — flush current
			if current != nil {
				individuals = append(individuals, *current)
				current = nil
			}
			inBirt = false

		case level == "1" && tag == "NAME" && current != nil:
			// Parse "First /Last/" format
			nameParts := strings.Split(value, "/")
			if len(nameParts) >= 1 {
				current.FirstName = strings.TrimSpace(nameParts[0])
			}
			if len(nameParts) >= 2 {
				current.LastName = strings.TrimSpace(nameParts[1])
			}

		case level == "1" && tag == "SEX" && current != nil:
			current.Sex = value

		case level == "1" && tag == "BIRT" && current != nil:
			inBirt = true

		case level == "1" && tag != "BIRT" && current != nil:
			inBirt = false

		case level == "2" && tag == "DATE" && current != nil && inBirt:
			current.BirthDate = value

		case level == "2" && tag == "PLAC" && current != nil && inBirt:
			current.BirthPlace = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan error: %w", err)
	}

	// Flush last record if still open
	if current != nil {
		individuals = append(individuals, *current)
	}

	return &ParseResult{
		Individuals: individuals,
		FamilyCount: familyCount,
	}, nil
}
