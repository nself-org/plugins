package trust

// Purpose: numeric ordering for TightenLower and TightenHigher keys, so a limit
// or a duration (an integer) tightens like an ordered enum does (P7-SCHED-02,
// lead ruling 2026-10-18: SCHED's limits, queue and cost keys are integers and
// the string-only order could not express them).
// Inputs: two policy values. Outputs: whether the second is as strict as the first.
// Constraints: a numeric key has an empty Order and an integer Default; only
// whole numbers count (a float, a string or a bool is never an integer); the
// enum path in stricter is unchanged.

import (
	"encoding/json"
	"strconv"
)

// IntValue returns v as an int64 when v is a whole number held as int, int64
// or json.Number.
func IntValue(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i, err := strconv.ParseInt(string(n), 10, 64)
		if err != nil {
			return 0, false
		}
		return i, true
	}
	return 0, false
}

// strictNumeric reports whether next is as strict as old for a numeric key.
// ok is false when either value is not a whole number.
func strictNumeric(kind Tightening, old, next any) (stricter, ok bool) {
	a, aok := IntValue(old)
	b, bok := IntValue(next)
	if !aok || !bok {
		return false, false
	}
	if kind == TightenLower {
		return b <= a, true
	}
	return b >= a, true
}
