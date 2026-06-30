package main

// main_test.go — GEDCOM parser tests for the family-gedcom plugin.
//
// Purpose: Verify that the GEDCOM 5.5.1 parser correctly extracts individuals
//          and families from a known fixture file.
//
// Inputs:  testdata/sample.ged — 2 individuals, 1 family.
// Outputs: ParseResult with 2 individuals parsed, family_count = 1.
// Constraints: no network, no DB — pure parse test.

import (
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/family-gedcom/internal/gedcom"
)

// TestParseGEDCOM_TwoIndividuals verifies that sample.ged yields exactly
// 2 individuals (the acceptance criterion for v0.0.1).
func TestParseGEDCOM_TwoIndividuals(t *testing.T) {
	path := filepath.Join("testdata", "sample.ged")

	result, err := gedcom.ParseGEDCOM(path)
	if err != nil {
		t.Fatalf("ParseGEDCOM() error: %v", err)
	}

	if len(result.Individuals) != 2 {
		t.Errorf("expected 2 individuals, got %d: %+v", len(result.Individuals), result.Individuals)
	}
}

// TestParseGEDCOM_FamilyCount verifies that sample.ged contains 1 family.
func TestParseGEDCOM_FamilyCount(t *testing.T) {
	path := filepath.Join("testdata", "sample.ged")

	result, err := gedcom.ParseGEDCOM(path)
	if err != nil {
		t.Fatalf("ParseGEDCOM() error: %v", err)
	}

	if result.FamilyCount != 1 {
		t.Errorf("expected 1 family, got %d", result.FamilyCount)
	}
}

// TestParseGEDCOM_IndividualFields verifies that name and sex are parsed correctly.
func TestParseGEDCOM_IndividualFields(t *testing.T) {
	path := filepath.Join("testdata", "sample.ged")

	result, err := gedcom.ParseGEDCOM(path)
	if err != nil {
		t.Fatalf("ParseGEDCOM() error: %v", err)
	}

	if len(result.Individuals) < 1 {
		t.Fatal("no individuals parsed")
	}

	john := result.Individuals[0]
	if john.FirstName != "John" {
		t.Errorf("expected FirstName 'John', got %q", john.FirstName)
	}
	if john.LastName != "Smith" {
		t.Errorf("expected LastName 'Smith', got %q", john.LastName)
	}
	if john.Sex != "M" {
		t.Errorf("expected Sex 'M', got %q", john.Sex)
	}
}

// TestParseGEDCOM_MissingFile verifies that a missing file returns an error.
func TestParseGEDCOM_MissingFile(t *testing.T) {
	_, err := gedcom.ParseGEDCOM("/nonexistent-file-xyz987.ged")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}
