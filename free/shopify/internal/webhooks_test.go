package internal

import (
	"encoding/json"
	"testing"
)

// TestJsonInt64_Float64 verifies conversion from float64 (standard JSON decode).
func TestJsonInt64_Float64(t *testing.T) {
	m := map[string]interface{}{"id": float64(12345)}
	got := jsonInt64(m, "id")
	if got != 12345 {
		t.Errorf("jsonInt64(float64) = %d, want 12345", got)
	}
}

// TestJsonInt64_JsonNumber verifies conversion from json.Number.
func TestJsonInt64_JsonNumber(t *testing.T) {
	m := map[string]interface{}{"id": json.Number("9876543210")}
	got := jsonInt64(m, "id")
	if got != 9876543210 {
		t.Errorf("jsonInt64(json.Number) = %d, want 9876543210", got)
	}
}

// TestJsonInt64_Missing verifies that a missing key returns 0.
func TestJsonInt64_Missing(t *testing.T) {
	m := map[string]interface{}{}
	got := jsonInt64(m, "nonexistent")
	if got != 0 {
		t.Errorf("jsonInt64(missing) = %d, want 0", got)
	}
}

// TestJsonInt64_WrongType verifies that a non-numeric value returns 0.
func TestJsonInt64_WrongType(t *testing.T) {
	m := map[string]interface{}{"id": "not-a-number"}
	got := jsonInt64(m, "id")
	if got != 0 {
		t.Errorf("jsonInt64(string) = %d, want 0", got)
	}
}

// TestJsonInt verifies the int-sized wrapper.
func TestJsonInt(t *testing.T) {
	m := map[string]interface{}{"count": float64(42)}
	got := jsonInt(m, "count")
	if got != 42 {
		t.Errorf("jsonInt = %d, want 42", got)
	}
}
