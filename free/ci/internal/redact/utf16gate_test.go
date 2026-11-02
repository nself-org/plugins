package redact

// Purpose: the UTF-16 gate treats every surrogate pair as a unit that can
// neither be skipped nor decode to an ASCII byte (D-1515). Check that over all
// supplementary runes, and that the BMP classes agree with the visible form.

import (
	"testing"
	"unicode"
)

func TestUnitClassSupplementaryRunes(t *testing.T) {
	for r := rune(0x10000); r <= unicode.MaxRune; r++ {
		if unicode.ToLower(r) < 0x80 || invisible(r) || unicode.IsSpace(r) {
			t.Fatalf("U+%X is skipped or folds to ASCII; surrogate pairs cannot all be class X", r)
		}
	}
}

// TestUnitClassBMP pins the assumption UTF16Candidate's soundness rests on:
// unicode.ToLower folds only U+0130 and U+212A to an ASCII byte. If a Go
// toolchain change adds another such rune this test fails on purpose; the gate
// builds its class table from ToLower, so review the gate before accepting it.
func TestUnitClassBMP(t *testing.T) {
	unitClass.once.Do(initUnitClass)
	var got []rune
	for u := 0x80; u < 1<<16; u++ {
		if unitClass.t[u] == clsG {
			got = append(got, rune(u))
		}
	}
	if len(got) != 2 || got[0] != 0x130 || got[1] != 0x212A {
		t.Fatalf("non-ASCII units that may decode to an ASCII byte: %U", got)
	}
}
